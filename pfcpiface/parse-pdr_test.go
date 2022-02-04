package main

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_CreatePortFilterCartesianProduct(t *testing.T) {
	type args struct {
		src portRange
		dst portRange
	}

	tests := []struct {
		name    string
		args    args
		want    []portFilterTernaryCartesianProduct
		wantErr bool
	}{
		{name: "exact ranges",
			args: args{src: newExactMatchPortFilter(5000), dst: newExactMatchPortFilter(80)},
			want: []portFilterTernaryCartesianProduct{{
				srcPort: 5000,
				srcMask: math.MaxUint16,
				dstPort: 80,
				dstMask: math.MaxUint16,
			}},
			wantErr: false},
		{name: "wildcard dst range",
			args: args{src: newExactMatchPortFilter(10), dst: newWildcardPortFilter()},
			want: []portFilterTernaryCartesianProduct{{
				srcPort: 10,
				srcMask: math.MaxUint16,
				dstPort: 0,
				dstMask: 0,
			}},
			wantErr: false},
		{name: "true range src range",
			args: args{src: newRangeMatchPortFilter(1, 3), dst: newExactMatchPortFilter(80)},
			want: []portFilterTernaryCartesianProduct{
				{
					srcPort: 0x1,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				},
				{
					srcPort: 0x2,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				},
				{
					srcPort: 0x3,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				}},
			wantErr: false},
		{name: "invalid double range",
			args:    args{src: newRangeMatchPortFilter(10, 20), dst: newRangeMatchPortFilter(80, 85)},
			want:    nil,
			wantErr: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := CreatePortFilterCartesianProduct(tt.args.src, tt.args.dst)
				if (err != nil) != tt.wantErr {
					t.Errorf("CreatePortFilterCartesianProduct() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("CreatePortFilterCartesianProduct() got = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_defaultPortFilter(t *testing.T) {
	t.Run("default constructed is wildcard", func(t *testing.T) {
		assert.True(t, portRange{}.isWildcardMatch(), "default portRange is wildcard")
	})
}

func Test_newWildcardPortFilter(t *testing.T) {
	tests := []struct {
		name string
		want portRange
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := newWildcardPortFilter(); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("newWildcardPortFilter() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_String(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want string
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.String(); got != tt.want {
					t.Errorf("String() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isExactMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isExactMatch(); got != tt.want {
					t.Errorf("isExactMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isRangeMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isRangeMatch(); got != tt.want {
					t.Errorf("isRangeMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isWildcardMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
		{name: "foo", pr: portRange{
			low:  0,
			high: 0,
		}, want: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isWildcardMatch(); got != tt.want {
					t.Errorf("isWildcardMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

// Perform a ternary match of value against rules.
func matchesTernary(value uint16, rules []portFilterTernaryRule) bool {
	for _, r := range rules {
		if (value & r.mask) == r.port {
			return true
		}
	}

	return false
}

func Test_portFilter_asComplexTernaryMatches(t *testing.T) {
	tests := []struct {
		name     string
		pr       portRange
		strategy RangeConversionStrategy
		wantErr  bool
		want     []portFilterTernaryRule
	}{
		{name: "Exact match port range",
			pr: portRange{
				low:  8888,
				high: 8888,
			},
			want: []portFilterTernaryRule{
				{port: 8888, mask: 0xffff},
			},
			wantErr: false},
		{name: "wildcard port range",
			pr: portRange{
				low:  0,
				high: math.MaxUint16,
			},
			want: []portFilterTernaryRule{
				{port: 0, mask: 0},
			},
			wantErr: false},
		{name: "Simplest port range",
			pr: portRange{
				low:  0b0, // 0
				high: 0b1, // 1
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b0, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "Simplest port range2",
			pr: portRange{
				low:  0b01, // 1
				high: 0b10, // 2
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xffff},
			//},
			wantErr: false},
		{name: "Trivial ternary port range",
			pr: portRange{
				low:  0x0100, // 256
				high: 0x01ff, // 511
			},
			strategy: Ternary,
			//want: []portFilterTernaryRule{
			//	{port: 0x0100, mask: 0xff00},
			//},
			wantErr: false},
		{name: "one to three range",
			pr: portRange{
				low:  0b01, // 1
				high: 0b11, // 3
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "True port range",
			pr: portRange{
				low:  0b00010, //  2
				high: 0b11101, // 29
			},
			wantErr: false},
		{name: "Worst case port range",
			pr: portRange{
				low:  1,
				high: 65534,
			},
			strategy: Ternary,
			wantErr:  false},
		{name: "low port filter",
			pr: portRange{
				low:  0,
				high: 1023,
			},
			strategy: Ternary,
			wantErr:  false},
		{name: "some small app filter",
			pr: portRange{
				low:  8080,
				high: 8084,
			},
			wantErr: false},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.pr.asComplexTernaryMatches(tt.strategy)
				if (err != nil) != tt.wantErr {
					t.Errorf("asComplexTernaryMatches() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
					t.Errorf("asComplexTernaryMatches() got = %v, want %v", got, tt.want)
				}
				// Do exhaustive test over entire value range.
				for port := 0; port <= math.MaxUint16; port++ {
					expectMatch := port >= int(tt.pr.low) && port <= int(tt.pr.high)
					if matchesTernary(uint16(port), got) != expectMatch {
						mod := " "
						if !expectMatch {
							mod = " not "
						}
						t.Errorf("Expected port %v to%vmatch against rules %v from range %+v", port, mod, got, tt.pr)
					}
				}
			},
		)
	}
}

func Test_portFilter_asTrivialTernaryMatch(t *testing.T) {
	tests := []struct {
		name     string
		pr       portRange
		wantPort uint16
		wantMask uint16
		wantErr  bool
	}{
		{name: "Wildcard range", pr: portRange{
			low:  0,
			high: 0,
		}, wantPort: 0, wantMask: 0, wantErr: false},
		{name: "Exact match range", pr: portRange{
			low:  100,
			high: 100,
		}, wantPort: 100, wantMask: 0xffff, wantErr: false},
		{name: "True range match fail", pr: portRange{
			low:  100,
			high: 200,
		}, wantPort: 0, wantMask: 0, wantErr: true},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.pr.asTrivialTernaryMatch()
				if (err != nil) != tt.wantErr {
					t.Errorf("asTrivialTernaryMatch() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				if got.port != tt.wantPort {
					t.Errorf("asTrivialTernaryMatch() got = %v, want %v", got.port, tt.wantPort)
				}
				if got.mask != tt.wantMask {
					t.Errorf("asTrivialTernaryMatch() got = %v, want %v", got.mask, tt.wantMask)
				}
			},
		)
	}
}

func Test_portFilter_Width(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want uint16
	}{
		{name: "wildcard", pr: newWildcardPortFilter(), want: math.MaxUint16},
		{name: "zero value", pr: portRange{}, want: math.MaxUint16},
		{name: "exact match", pr: newExactMatchPortFilter(100), want: 1},
		{name: "range match", pr: newRangeMatchPortFilter(10, 12), want: 3},
		{name: "range single match", pr: newRangeMatchPortFilter(1000, 1000), want: 1},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.Width(); got != tt.want {
					t.Errorf("Width() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}
