package types

import "testing"

func TestParamTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		pt   ParamType
		want string
	}{
		{"string", TypeString, "str"},
		{"int", TypeInt, "int"},
		{"float", TypeFloat, "float"},
		{"boolean", TypeBoolean, "boolean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.pt); got != tt.want {
				t.Errorf("ParamType(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsBoolFlag(t *testing.T) {
	tests := []struct {
		name string
		p    ParamDef
		want bool
	}{
		{
			name: "boolean flag",
			p:    ParamDef{Type: TypeBoolean},
			want: true,
		},
		{
			name: "string param",
			p:    ParamDef{Type: TypeString},
			want: false,
		},
		{
			name: "int param",
			p:    ParamDef{Type: TypeInt},
			want: false,
		},
		{
			name: "float param",
			p:    ParamDef{Type: TypeFloat},
			want: false,
		},
		{
			name: "zero-value ParamDef",
			p:    ParamDef{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.IsBoolFlag(); got != tt.want {
				t.Errorf("ParamDef{Type: %q}.IsBoolFlag() = %v, want %v",
					tt.p.Type, got, tt.want)
			}
		})
	}
}
