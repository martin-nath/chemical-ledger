package utils

// Chemical represents a chemical with its net stock.
type Chemical struct {
	ID       string `json:"id"` // This will be the chemical name in CamelCase.
	Name     string `json:"name"`
	NetStock int    `json:"net_stock"` // Current stock (incoming adds, outgoing subtracts)
}

// Transaction represents a chemical transaction.
type Filters struct {
	Type         string `json:"type"`
	FromDate     string `json:"from_date"`
	ToDate       string `json:"to_date"`
	CompoundName string `json:"compound_name"`
}

type Entry struct {
	ID              string `json:"entry_id"`
	Type            string `json:"type"`
	Date            string `json:"date"`
	Remark          string `json:"remark"`
	VoucherNo       string `json:"voucher_no"`
	NetStock        int    `json:"net_stock"`
	CompoundId      string `json:"compound_id"`
	NumOfUnits      int    `json:"num_of_units"`
	QuantityPerUnit int    `json:"quantity_per_unit"`
}

type UpdatedEntry struct {
	Type            string `json:"type"`
	NumOfUnits      int    `json:"num_of_units"`
	QuantityPerUnit int    `json:"quantity_per_unit"`
	NetStock        int    `json:"net_stock"`
	Remark          string `json:"remark"`
	VoucherNo       string `json:"voucher_no"`
}

type Resp struct {
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}
