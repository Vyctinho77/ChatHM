package conversations

import "testing"

func TestNormalizeTaxID(t *testing.T) {
	ok := map[string]string{
		"529.982.247-25":     "52998224725",
		"52998224725":        "52998224725",
		"11.222.333/0001-81": "11222333000181",
		"11222333000181":     "11222333000181",
		"  ":                 "",
		"":                   "",
	}
	for in, want := range ok {
		if got, err := NormalizeTaxID(in); err != nil || got != want {
			t.Errorf("%q: %q %v, esperava %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"529.982.247-26", "11222333000182", "11111111111", "00000000000000", "123", "abc", "5299822472"} {
		if got, err := NormalizeTaxID(bad); err == nil {
			t.Errorf("%q deveria ser recusado (virou %q)", bad, got)
		}
	}
}

func TestNormalizeAsaasID(t *testing.T) {
	if got, err := NormalizeAsaasID(" cus_000005401844 "); err != nil || got != "cus_000005401844" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := NormalizeAsaasID(""); err != nil || got != "" {
		t.Fatalf("vazio apaga: %q %v", got, err)
	}
	for _, bad := range []string{"000005401844", "cus_", "cus_12", "pay_000005401844", "cus_00 0005"} {
		if _, err := NormalizeAsaasID(bad); err == nil {
			t.Errorf("%q deveria ser recusado", bad)
		}
	}
}

func TestFormatTaxID(t *testing.T) {
	if got := FormatTaxID("52998224725"); got != "529.982.247-25" {
		t.Fatal(got)
	}
	if got := FormatTaxID("11222333000181"); got != "11.222.333/0001-81" {
		t.Fatal(got)
	}
	if got := FormatTaxID("123"); got != "123" {
		t.Fatal(got)
	}
}
