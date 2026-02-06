package syntax

import "testing"

func TestPosString(t *testing.T) {
	tests := []struct {
		name     string
		pos      Pos
		wantStr  string
	}{
		{
			name:    "with filename",
			pos:     NewPos("test.yoru", 10, 5),
			wantStr: "test.yoru:10:5",
		},
		{
			name:    "without filename",
			pos:     NewPos("", 10, 5),
			wantStr: "10:5",
		},
		{
			name:    "line 1 col 1",
			pos:     NewPos("main.yoru", 1, 1),
			wantStr: "main.yoru:1:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.String(); got != tt.wantStr {
				t.Errorf("Pos.String() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

func TestPosIsValid(t *testing.T) {
	tests := []struct {
		name  string
		pos   Pos
		valid bool
	}{
		{
			name:  "valid position",
			pos:   NewPos("test.yoru", 1, 1),
			valid: true,
		},
		{
			name:  "valid position line 100",
			pos:   NewPos("", 100, 50),
			valid: true,
		},
		{
			name:  "invalid - zero line",
			pos:   NewPos("test.yoru", 0, 1),
			valid: false,
		},
		{
			name:  "invalid - zero value",
			pos:   Pos{},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.IsValid(); got != tt.valid {
				t.Errorf("Pos.IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestPosGetters(t *testing.T) {
	pos := NewPos("test.yoru", 42, 13)

	if got := pos.Line(); got != 42 {
		t.Errorf("Pos.Line() = %d, want 42", got)
	}

	if got := pos.Col(); got != 13 {
		t.Errorf("Pos.Col() = %d, want 13", got)
	}

	if got := pos.Filename(); got != "test.yoru" {
		t.Errorf("Pos.Filename() = %q, want %q", got, "test.yoru")
	}
}

func TestNewPos(t *testing.T) {
	pos := NewPos("file.yoru", 5, 10)

	if pos.filename != "file.yoru" {
		t.Errorf("filename = %q, want %q", pos.filename, "file.yoru")
	}
	if pos.line != 5 {
		t.Errorf("line = %d, want 5", pos.line)
	}
	if pos.col != 10 {
		t.Errorf("col = %d, want 10", pos.col)
	}
}
