package ipset

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCNPlainCIDRsCanonicalizesAndDropsContained(t *testing.T) {
	got, err := ParseCN(strings.NewReader(`
# manually maintained additions
1.2.0.0/16
1.2.3.0/24
1.2.0.0/16
240e::/20
`))
	if err != nil {
		t.Fatal(err)
	}
	v4, v6 := SplitFamilies(got)
	if !reflect.DeepEqual(v4, []string{"1.2.0.0/16"}) {
		t.Fatalf("IPv4 = %v", v4)
	}
	if !reflect.DeepEqual(v6, []string{"240e::/20"}) {
		t.Fatalf("IPv6 = %v", v6)
	}
}

func TestParseCNDelegatedFileSkipsOtherCountriesAndResources(t *testing.T) {
	got, err := ParseCN(strings.NewReader(`
apnic|CN|ipv4|1.2.0.0|65536|20110414|allocated
apnic|JP|ipv4|1.3.0.0|65536|20110414|allocated
apnic|CN|asn|45102|1|20110414|allocated
apnic|CN|ipv6|240e::|20|20110414|allocated
apnic|CN|ipv4|203.0.113.0|256|20110414|reserved
`))
	if err != nil {
		t.Fatal(err)
	}
	v4, v6 := SplitFamilies(got)
	if !reflect.DeepEqual(v4, []string{"1.2.0.0/16"}) {
		t.Fatalf("IPv4 = %v", v4)
	}
	if !reflect.DeepEqual(v6, []string{"240e::/20"}) {
		t.Fatalf("IPv6 = %v", v6)
	}
}

func TestParseCNRejectsPrivateOrDefaultManualPrefixes(t *testing.T) {
	for _, input := range []string{"0.0.0.0/0\n", "10.0.0.0/8\n", "fc00::/7\n"} {
		if _, err := ParseCN(strings.NewReader(input)); err == nil {
			t.Fatalf("ParseCN(%q) unexpectedly succeeded", input)
		}
	}
}

func TestParseCNRejectsMalformedDelegatedIPv4Range(t *testing.T) {
	_, err := ParseCN(strings.NewReader("apnic|CN|ipv4|1.2.3.0|512|20110414|allocated\n"))
	if err == nil || !strings.Contains(err.Error(), "prefix-aligned") {
		t.Fatalf("err = %v", err)
	}
}
