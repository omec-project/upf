package main

import (
	"math"
	"reflect"
	"testing"
)

func Test_convertPortFiltersToTernary(t *testing.T) {
	type args struct {
		src portFilter
		dst portFilter
	}
	tests := []struct {
		name    string
		args    args
		want    []portFilterTernaryCrossProduct
		wantErr bool
	}{
		{name: "exact ranges",
			args: args{src: newExactMatchPortFilter(5000), dst: newExactMatchPortFilter(80)},
			want: []portFilterTernaryCrossProduct{{
				srcPort: 5000,
				srcMask: math.MaxUint16,
				dstPort: 80,
				dstMask: math.MaxUint16,
			}},
			wantErr: false},
		{name: "wildcard dst range",
			args: args{src: newExactMatchPortFilter(10), dst: newWildcardPortFilter()},
			want: []portFilterTernaryCrossProduct{{
				srcPort: 10,
				srcMask: math.MaxUint16,
				dstPort: 0,
				dstMask: 0,
			}},
			wantErr: false},
		{name: "true range src range",
			args: args{src: newRangeMatchPortFilter(1, 3), dst: newExactMatchPortFilter(80)},
			want: []portFilterTernaryCrossProduct{
				{
					srcPort: 0x1,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				},
				{
					srcPort: 0x2,
					srcMask: 0xfffe,
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
				got, err := convertPortFiltersToTernary(tt.args.src, tt.args.dst)
				if (err != nil) != tt.wantErr {
					t.Errorf("convertPortFiltersToTernary() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("convertPortFiltersToTernary() got = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_newWildcardPortFilter(t *testing.T) {
	tests := []struct {
		name string
		want portFilter
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
	type fields struct {
		PortLow  uint16
		PortHigh uint16
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				pr := portFilter{
					portLow:  tt.fields.PortLow,
					portHigh: tt.fields.PortHigh,
				}
				if got := pr.String(); got != tt.want {
					t.Errorf("String() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isExactMatch(t *testing.T) {
	type fields struct {
		PortLow  uint16
		PortHigh uint16
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				pr := portFilter{
					portLow:  tt.fields.PortLow,
					portHigh: tt.fields.PortHigh,
				}
				if got := pr.isExactMatch(); got != tt.want {
					t.Errorf("isExactMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isRangeMatch(t *testing.T) {
	type fields struct {
		PortLow  uint16
		PortHigh uint16
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				pr := portFilter{
					portLow:  tt.fields.PortLow,
					portHigh: tt.fields.PortHigh,
				}
				if got := pr.isRangeMatch(); got != tt.want {
					t.Errorf("isRangeMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portFilter_isWildcardMatch(t *testing.T) {
	type fields struct {
		PortLow  uint16
		PortHigh uint16
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		// TODO: Add test cases.
		{name: "foo", fields: fields{
			PortLow:  0,
			PortHigh: 0,
		}, want: true},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				pr := portFilter{
					portLow:  tt.fields.PortLow,
					portHigh: tt.fields.PortHigh,
				}
				if got := pr.isWildcardMatch(); got != tt.want {
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
		name    string
		pr      portFilter
		wantErr bool
	}{
		{name: "Exact match port range",
			pr: portFilter{
				portLow:  8888,
				portHigh: 8888,
			},
			//want: []portFilterTernaryRule{
			//	{port: 8888, mask: 0xffff},
			//},
			wantErr: false},
		{name: "wildcard port range",
			pr: portFilter{
				portLow:  0,
				portHigh: math.MaxUint16,
			},
			wantErr: false},
		{name: "Simplest port range",
			pr: portFilter{
				portLow:  0b0, // 0
				portHigh: 0b1, // 1
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b0, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "Simplest port range2",
			pr: portFilter{
				portLow:  0b01, // 1
				portHigh: 0b10, // 2
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xffff},
			//},
			wantErr: false},
		{name: "Trivial ternary port range",
			pr: portFilter{
				portLow:  0x0100, // 256
				portHigh: 0x01ff, // 511
			},
			//want: []portFilterTernaryRule{
			//	{port: 0x0100, mask: 0xff00},
			//},
			wantErr: false},
		{name: "one to three range",
			pr: portFilter{
				portLow:  0b01, // 1
				portHigh: 0b11, // 3
			},
			//want: []portFilterTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "True port range",
			pr: portFilter{
				portLow:  0b00010, //  2
				portHigh: 0b11101, // 29
			},
			wantErr: false},
		{name: "low port filter",
			pr: portFilter{
				portLow:  0,
				portHigh: 1023,
			},
			wantErr: false},
		{name: "some app filter",
			pr: portFilter{
				portLow:  8080,
				portHigh: 8084,
			},
			wantErr: false},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				pr := portFilter{
					portLow:  tt.pr.portLow,
					portHigh: tt.pr.portHigh,
				}
				got, err := pr.asComplexTernaryMatches()
				if (err != nil) != tt.wantErr {
					t.Errorf("asComplexTernaryMatches() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				//if !reflect.DeepEqual(got, tt.want) {
				//	t.Errorf("asComplexTernaryMatches() got = %v, want %v", got, tt.want)
				//}
				// Do exhaustive test over entire value range.
				for port := 0; port <= math.MaxUint16; port++ {
					expectMatch := port >= int(tt.pr.portLow) && port <= int(tt.pr.portHigh)
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
		pr       portFilter
		wantPort uint16
		wantMask uint16
		wantErr  bool
	}{
		{name: "Wildcard range", pr: portFilter{
			portLow:  0,
			portHigh: 0,
		}, wantPort: 0, wantMask: 0, wantErr: false},
		{name: "Exact match range", pr: portFilter{
			portLow:  100,
			portHigh: 100,
		}, wantPort: 100, wantMask: 0xffff, wantErr: false},
		{name: "True range match fail", pr: portFilter{
			portLow:  100,
			portHigh: 200,
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
