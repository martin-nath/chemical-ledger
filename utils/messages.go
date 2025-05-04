package utils

const (
	REQUEST_BODY_DECODE_ERR = "We encountered an issue reading the data you sent."

	MISSING_REQUIRED_FIELDS = "Please make sure all necessary information is provided."
	INVALID_ENTRY_TYPE      = "The type of entry you provided is not valid."
	INVALID_DATE_FORMAT     = "Please enter the date in the format YYYY-MM-DD."
	FUTURE_DATE_ERR         = "The date you entered is in the future."
	INVALID_DATE_RANGE      = "The provided date range is not valid."

	INVALID_COMPOUND_ID          = "The provided compound identifier is not valid."
	COMPOUND_ALREADY_EXISTS      = "A compound with this identifier already exists."
	INVALID_COMPOUND_FILTER_TYPE = "The way you are trying to filter compounds is not valid."

	INVALID_ENTRY_ID = "The provided entry identifier is not valid."

	INVALID_SCALE_ERR = "The provided scale is not valid."

	TX_START_ERR           = "An error occurred while starting the process."
	COMMIT_TRANSACTION_ERR = "There was an error saving the changes."
	INVALID_TRANSACTIONS_TYPE = "The provided transactions type is not valid."

	COMPOUND_ID_CHECK_ERR  = "We couldn't verify the compound identifier."
	COMPOUND_RETRIEVAL_ERR = "We encountered an issue retrieving the list of compounds."
	COMPOUND_UPDATE_ERR    = "We were unable to update the compound information."
	INSERT_COMPOUND_ERR    = "We were unable to add the new compound."
	COMPOUND_SCALE_ERR     = "We were unable to update the compound scale."

	INSERT_QUANTITY_ERR   = "There was a problem recording the quantity."
	INSERT_ENTRY_ERR      = "We were unable to save the entry."
	UPDATE_ENTRY_ERR      = "We couldn't update the entry information."
	ENTRY_UPDATE_SCAN_ERR = "An issue occurred while processing the entry update."
	SUBSEQUENT_UPDATE_ERR = "We couldn't update related entries."
	ENTRY_RETRIEVAL_ERR   = "We were unable to find the requested entry."

	STOCK_RETRIEVAL_ERR    = "We couldn't retrieve the current stock information."
	INSUFFICIENT_STOCK_ERR = "There is not enough stock available for the requested outgoing entry."

	NO_ERR = ""
)
