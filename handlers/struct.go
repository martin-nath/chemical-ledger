package handlers

import (
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func toCamelCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = cases.Title(language.Und, cases.NoLower).String(w)
	}
	return strings.Join(words, "")
}

// Chemical represents a chemical with its net stock.
type Chemical struct {
	ID       string `json:"id"` // This will be the chemical name in CamelCase.
	Name     string `json:"name"`
	NetStock int    `json:"net_stock"` // Current stock (incoming adds, outgoing subtracts)
}

// Transaction represents a chemical transaction.
type Filters struct {
	Type            string `json:"type"` // "Incoming" or "Outgoing"
	FromDate        string `json:"from_date"`
	ToDate          string `json:"to_date"`
	CompoundName    string `json:"compound_name"` // Original chemical name (for display)
}

type Entry struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Date            string `json:"date"`
	Remark          string `json:"remark"`
	VoucherNo       string `json:"voucher_no"`
	NetStock        int    `json:"net_stock"`
	CompoundName    string `json:"compound_name"`
	Scale           string `json:"scale"`
	NumOfUnits      int    `json:"num_of_units"`
	QuantityPerUnit int    `json:"quantity_per_unit"`
}
