/*
 * Yoru Runtime Implementation
 *
 * This is a minimal runtime focused on correctness, not performance.
 * Uses direct malloc/free with a linked list for GC root tracking.
 */

#include "runtime.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <execinfo.h>

/*
 * =============================================================================
 * Global State
 * =============================================================================
 */

/* Head of the allocated objects list */
static Object* alloc_list = NULL;

/* Runtime statistics */
static RuntimeStats stats = {0};

/* GC threshold: collect when this many bytes are allocated */
static uint64_t gc_threshold = 1024 * 1024;  /* 1 MB initial threshold */

/* Current allocated bytes since last GC */
static uint64_t bytes_since_gc = 0;

/* Verbose GC flag (set by environment variable YORU_GC_VERBOSE) */
static int gc_verbose = 0;

/* Automatic GC enable flag (YORU_GC_ENABLE=1) */
static int gc_enabled = 0;

/* GC stress mode (YORU_GC_STRESS=1) */
static int gc_stress = 0;

/* LLVM GC root chain (defined by LLVM, we just declare it) */
struct StackEntry* llvm_gc_root_chain = NULL;

/*
 * =============================================================================
 * Built-in Type Descriptors
 * =============================================================================
 */

const TypeDesc rt_type_int = {
    .size = 8,
    .num_ptrs = 0,
    .offsets = NULL,
};

const TypeDesc rt_type_float = {
    .size = 8,
    .num_ptrs = 0,
    .offsets = NULL,
};

const TypeDesc rt_type_bool = {
    .size = 1,
    .num_ptrs = 0,
    .offsets = NULL,
};

/*
 * String data is not GC-managed in the current learning subset,
 * so it does not contribute GC offsets.
 */
const TypeDesc rt_type_string = {
    .size = 16,  /* ptr (8) + len (8) */
    .num_ptrs = 0,
    .offsets = NULL,
};

/*
 * =============================================================================
 * Runtime Initialization
 * =============================================================================
 */

void rt_init(void) {
    /* Check for verbose GC mode */
    const char* verbose = getenv("YORU_GC_VERBOSE");
    if (verbose && (strcmp(verbose, "1") == 0 || strcmp(verbose, "true") == 0)) {
        gc_verbose = 1;
    }

    /* Enable automatic GC only when explicitly requested */
    const char* enable = getenv("YORU_GC_ENABLE");
    if (enable && (strcmp(enable, "1") == 0 || strcmp(enable, "true") == 0)) {
        gc_enabled = 1;
    }

    /* Stress mode: collect on every allocation */
    const char* stress = getenv("YORU_GC_STRESS");
    if (stress && (strcmp(stress, "1") == 0 || strcmp(stress, "true") == 0)) {
        gc_stress = 1;
        gc_enabled = 1;
        gc_threshold = 0;
    }

    /* Reset statistics */
    memset(&stats, 0, sizeof(stats));

    if (gc_verbose) {
        fprintf(stderr, "[GC] Runtime initialized\n");
    }
}

void rt_shutdown(void) {
    /* Free all remaining objects */
    Object* obj = alloc_list;
    while (obj) {
        Object* next = OBJ_NEXT(obj);
        free(obj);
        obj = next;
    }
    alloc_list = NULL;

    if (gc_verbose) {
        fprintf(stderr, "[GC] Runtime shutdown. Final stats:\n");
        rt_print_stats();
    }
}

/*
 * =============================================================================
 * Memory Allocation
 * =============================================================================
 */

void* rt_alloc(uint64_t size, const TypeDesc* type) {
    /* Check if we should trigger GC */
    uint64_t alloc_size = sizeof(ObjHeader) + size;
    bytes_since_gc += alloc_size;

    if (gc_enabled && bytes_since_gc >= gc_threshold) {
        rt_collect();
    }

    /* Allocate memory */
    Object* obj = (Object*)malloc(alloc_size);
    if (!obj) {
        rt_panic("out of memory");
    }

    /* Initialize header */
    obj->header.type = type;
    obj->header.next_mark = (uintptr_t)alloc_list;  /* link to list, mark=0 */

    if (type && type->size != size) {
        rt_panic("rt_alloc size mismatch");
    }

    /* Zero-initialize the data area */
    memset(obj->data, 0, size);

    /* Add to allocation list */
    alloc_list = obj;

    /* Update statistics */
    stats.alloc_count++;
    stats.live_objects++;
    stats.heap_size += alloc_size;

    if (gc_verbose) {
        fprintf(stderr, "[GC] Allocated %llu bytes at %p\n",
                (unsigned long long)size, (void*)obj->data);
    }

    return obj->data;
}

/*
 * =============================================================================
 * Garbage Collection - Mark Phase
 * =============================================================================
 */

/* Forward declaration */
static void mark_object(void* ptr);

/* Mark an object and recursively mark its pointer fields */
static void mark_object(void* ptr) {
    if (!ptr) return;

    Object* obj = DATA_TO_OBJ(ptr);

    /* Already marked? */
    if (OBJ_MARKED(obj)) return;

    /* Mark this object */
    OBJ_SET_MARK(obj);

    /* Get type descriptor */
    const TypeDesc* type = obj->header.type;
    if (!type) return;

    /* Recursively mark pointer fields */
    for (size_t i = 0; i < type->num_ptrs; i++) {
        uint32_t offset = type->offsets[i];
        void** field_ptr = (void**)(obj->data + offset);
        void* field_val = *field_ptr;
        if (field_val) {
            mark_object(field_val);
        }
    }
}

/* Mark all roots from LLVM's shadow stack */
static void mark_roots(void) {
    struct StackEntry* entry = llvm_gc_root_chain;

    while (entry) {
        const struct FrameMap* map = entry->map;
        if (map) {
            for (int32_t i = 0; i < map->num_roots; i++) {
                void** slot = (void**)entry->roots[i];
                if (slot && *slot) {
                    mark_object(*slot);
                }
            }
        }
        entry = entry->next;
    }
}

/*
 * =============================================================================
 * Garbage Collection - Sweep Phase
 * =============================================================================
 */

static void sweep(void) {
    Object* prev_obj = NULL;
    Object* obj = alloc_list;
    uint64_t freed = 0;

    while (obj) {
        Object* next = OBJ_NEXT(obj);

        if (OBJ_MARKED(obj)) {
            /* Object is alive, clear mark for next cycle */
            OBJ_CLEAR_MARK(obj);
            prev_obj = obj;
        } else {
            /* Object is dead, remove from list and free */
            if (prev_obj == NULL) {
                alloc_list = next;
            } else {
                OBJ_SET_NEXT(prev_obj, next);
            }

            uint64_t obj_size = sizeof(ObjHeader) + obj->header.type->size;
            stats.heap_size -= obj_size;
            stats.live_objects--;
            freed++;

            if (gc_verbose) {
                fprintf(stderr, "[GC] Freed object at %p (size=%llu)\n",
                        (void*)obj->data,
                        (unsigned long long)obj->header.type->size);
            }

            free(obj);
        }

        obj = next;
    }

    stats.freed_count += freed;
}

/*
 * =============================================================================
 * Garbage Collection - Main Entry
 * =============================================================================
 */

void rt_collect(void) {
    if (gc_verbose) {
        fprintf(stderr, "[GC] Starting collection #%llu (heap=%llu bytes, live=%llu)\n",
                (unsigned long long)(stats.gc_count + 1),
                (unsigned long long)stats.heap_size,
                (unsigned long long)stats.live_objects);
    }

    /* Mark phase */
    mark_roots();

    /* Sweep phase */
    uint64_t live_before = stats.live_objects;
    sweep();
    uint64_t freed = live_before - stats.live_objects;

    /* Update statistics */
    stats.gc_count++;
    bytes_since_gc = 0;

    /* Adjust threshold based on live data */
    /* New threshold = 2 * current heap size, minimum 1MB */
    gc_threshold = stats.heap_size * 2;
    if (gc_threshold < 1024 * 1024) {
        gc_threshold = 1024 * 1024;
    }

    if (gc_verbose) {
        fprintf(stderr, "[GC] Collection done. Freed %llu objects, %llu remain\n",
                (unsigned long long)freed,
                (unsigned long long)stats.live_objects);
    }
}

/*
 * =============================================================================
 * Error Handling
 * =============================================================================
 */

/* Print a simple stack trace */
static void print_stack_trace(void) {
    void* buffer[64];
    int nptrs = backtrace(buffer, 64);
    char** symbols = backtrace_symbols(buffer, nptrs);

    if (symbols) {
        fprintf(stderr, "\nStack trace:\n");
        for (int i = 2; i < nptrs; i++) {  /* Skip rt_panic frames */
            fprintf(stderr, "  %s\n", symbols[i]);
        }
        free(symbols);
    }
}

void rt_panic(const char* msg) {
    fprintf(stderr, "panic: %s\n", msg);
    print_stack_trace();
    rt_print_stats();
    exit(1);
}

void rt_panic_string(YoruString msg) {
    fprintf(stderr, "panic: %.*s\n", (int)msg.len, msg.ptr);
    print_stack_trace();
    rt_print_stats();
    exit(1);
}

/*
 * =============================================================================
 * I/O Functions
 * =============================================================================
 */

void rt_print_i64(int64_t x) {
    printf("%lld", (long long)x);
}

void rt_print_f64(double x) {
    printf("%g", x);
}

void rt_print_bool(int8_t b) {
    printf("%s", b ? "true" : "false");
}

void rt_print_string(YoruString s) {
    printf("%.*s", (int)s.len, s.ptr);
}

void rt_println(void) {
    printf("\n");
}

/*
 * =============================================================================
 * Bounds Checking
 * =============================================================================
 */

void rt_bounds_check(int64_t index, int64_t len) {
    if (index < 0 || index >= len) {
        char buf[128];
        snprintf(buf, sizeof(buf),
                 "index out of range [%lld] with length %lld",
                 (long long)index, (long long)len);
        rt_panic(buf);
    }
}

/*
 * =============================================================================
 * Runtime Statistics
 * =============================================================================
 */

RuntimeStats rt_get_stats(void) {
    return stats;
}

void rt_print_stats(void) {
    fprintf(stderr, "\n=== Runtime Statistics ===\n");
    fprintf(stderr, "  Allocations:   %llu\n", (unsigned long long)stats.alloc_count);
    fprintf(stderr, "  GC cycles:     %llu\n", (unsigned long long)stats.gc_count);
    fprintf(stderr, "  Live objects:  %llu\n", (unsigned long long)stats.live_objects);
    fprintf(stderr, "  Heap size:     %llu bytes\n", (unsigned long long)stats.heap_size);
    fprintf(stderr, "  Freed total:   %llu\n", (unsigned long long)stats.freed_count);
    fprintf(stderr, "==========================\n");
}

/*
 * =============================================================================
 * Main Entry Point (for standalone testing)
 * =============================================================================
 */

/* The compiler generates a yoru_main function */
extern void yoru_main(void);

/* Weak symbol - allows linking without yoru_main for testing */
__attribute__((weak))
void yoru_main(void) {
    fprintf(stderr, "No yoru_main defined\n");
}

#ifndef YORU_NO_MAIN
int main(int argc, char** argv) {
    (void)argc;
    (void)argv;

    rt_init();
    yoru_main();
    rt_shutdown();

    return 0;
}
#endif
