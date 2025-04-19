package utils

// Chemical represents a chemical with its net stock.
type Chemical struct {
	ID       string `json:"id"` // This will be the chemical name in CamelCase.
	Name     string `json:"name"`
	NetStock int    `json:"net_stock"` // Current stock (incoming adds, outgoing subtracts)
}

// Quantity represents the quantity details of an entry.
type Quantity struct {
	ID              string `json:"id"`
	NumOfUnits      int    `json:"num_of_units"`
	QuantityPerUnit int    `json:"quantity_per_unit"`
}

// Filters represents the filtering options for retrieving transactions.
type Filters struct {
	Type         string `json:"type"`
	FromDate     string `json:"from_date"`
	ToDate       string `json:"to_date"`
	CompoundName string `json:"compound_name"`
	Page         int    `json:"page,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

// Entry represents a stock entry.
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

// UpdatedEntry represents the fields that can be updated for an entry.
type UpdatedEntry struct {
	Type            string `json:"type,omitempty"`
	NumOfUnits      int    `json:"num_of_units,omitempty"`
	QuantityPerUnit int    `json:"quantity_per_unit,omitempty"`
	NetStock        int    `json:"net_stock,omitempty"`
	Remark          string `json:"remark,omitempty"`
	VoucherNo       string `json:"voucher_no,omitempty"`
	Date            string `json:"date,omitempty"`
	CompoundId      string `json:"compound_id,omitempty"`
}

// Resp represents a standard JSON response.
type Resp struct {
	Error   string `json:"error,omitempty"`
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

// Compound represents a chemical compound stored in the database.
type Compound struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
