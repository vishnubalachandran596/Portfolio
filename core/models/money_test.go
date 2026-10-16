package models

import "testing"

func TestFormatMoneyGrouping(t *testing.T) {
	cases := []struct {
		cents    int
		currency string
		want     string
	}{
		{49900, CurrencyINR, "₹499"},
		{500000, CurrencyINR, "₹5,000"},
		{15000000, CurrencyINR, "₹1,50,000"},
		{1234567800, CurrencyINR, "₹1,23,45,678"},
		{300000, "USD", "$3,000"},
		{12345678, "USD", "$123,456.78"},
		{0, CurrencyINR, "₹0"},
	}
	for _, c := range cases {
		if got := FormatMoney(c.cents, c.currency); got != c.want {
			t.Errorf("FormatMoney(%d, %s) = %q, want %q", c.cents, c.currency, got, c.want)
		}
	}
}
