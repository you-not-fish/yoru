#include <stdint.h>
#include <stddef.h>
#include <stdio.h>

typedef struct Point {
    int64_t x;
    int64_t y;
} Point;

typedef struct Mixed {
    uint8_t b;
    int64_t i;
    void* p;
    double f;
} Mixed;

static size_t field_align_int64(void) { return _Alignof(int64_t); }
static size_t field_align_bool(void) { return _Alignof(uint8_t); }
static size_t field_align_ptr(void) { return _Alignof(void*); }
static size_t field_align_double(void) { return _Alignof(double); }

int main(void) {
    printf("struct Point size=%zu align=%zu\n", sizeof(Point), _Alignof(Point));
    printf("  field x offset=%zu size=%zu align=%zu\n",
           offsetof(Point, x), sizeof(((Point*)0)->x), field_align_int64());
    printf("  field y offset=%zu size=%zu align=%zu\n",
           offsetof(Point, y), sizeof(((Point*)0)->y), field_align_int64());

    printf("struct Mixed size=%zu align=%zu\n", sizeof(Mixed), _Alignof(Mixed));
    printf("  field b offset=%zu size=%zu align=%zu\n",
           offsetof(Mixed, b), sizeof(((Mixed*)0)->b), field_align_bool());
    printf("  field i offset=%zu size=%zu align=%zu\n",
           offsetof(Mixed, i), sizeof(((Mixed*)0)->i), field_align_int64());
    printf("  field p offset=%zu size=%zu align=%zu\n",
           offsetof(Mixed, p), sizeof(((Mixed*)0)->p), field_align_ptr());
    printf("  field f offset=%zu size=%zu align=%zu\n",
           offsetof(Mixed, f), sizeof(((Mixed*)0)->f), field_align_double());

    return 0;
}
