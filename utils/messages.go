package utils

const (
	REQUEST_BODY_DECODE_ERR = "Unable to read the request body. Ensure the data format is correct."

	TRIAL_PERIOD_LIMIT_EXCEEDED = "Trial period limit exceeded. Please contact the developers."

	MISSING_REQUIRED_FIELDS = "Required fields are missing. Complete all necessary fields and try again."
	INVALID_ENTRY_TYPE      = "Unrecognized entry type. Use a valid entry type."
	INVALID_DATE_FORMAT     = "Invalid date format. Use the format YYYY-MM-DD."
	FUTURE_DATE_ERR         = "The selected date is in the future. Use a current or past date."
	INVALID_DATE_RANGE      = "Invalid date range. Check the start and end dates."

	INVALID_COMPOUND_ID          = "Compound ID does not match any existing records."
	COMPOUND_ALREADY_EXISTS      = "A compound with the same name already exists. Use a different name."
	INVALID_COMPOUND_FILTER_TYPE = "Invalid filter type for compound. Check available filter options."

	INVALID_ENTRY_ID = "Entry ID not found in records."

	INVALID_SCALE_ERR = "Provided scale value is invalid."

	TX_START_ERR              = "Transaction could not be started."
	COMMIT_TRANSACTION_ERR    = "Transaction could not be committed."
	INVALID_TRANSACTIONS_TYPE = "Invalid transaction type specified."

	COMPOUND_ID_CHECK_ERR  = "Compound ID could not be verified."
	COMPOUND_RETRIEVAL_ERR = "Failed to retrieve compound data."
	COMPOUND_UPDATE_ERR    = "Compound data could not be updated."
	INSERT_COMPOUND_ERR    = "Failed to insert compound data."
	COMPOUND_SCALE_ERR     = "Failed to update compound scale."

	INSERT_QUANTITY_ERR   = "Failed to insert quantity data."
	INSERT_ENTRY_ERR      = "Failed to insert entry data."
	UPDATE_ENTRY_ERR      = "Failed to update entry data."
	ENTRY_UPDATE_SCAN_ERR = "Error occurred while scanning updated entry data."
	SUBSEQUENT_UPDATE_ERR = "Failed to update subsequent entries."
	ENTRY_RETRIEVAL_ERR   = "Entry data could not be retrieved."

	STOCK_RETRIEVAL_ERR    = "Failed to retrieve stock data."
	INSUFFICIENT_STOCK_ERR = "Insufficient stock for the requested transaction."

	NO_ERR = ""
)
