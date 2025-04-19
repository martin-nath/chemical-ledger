package utils

const (
	InvalidMethod                        = "This action is not permitted with the current method. Please verify and try again."
	Req_body_decode_error                = "We encountered an issue interpreting the data you provided. Please review the information and resubmit."
	MissingFields_or_inappropriate_value = "Please ensure all necessary fields are completed accurately."
	Invalid_date_format                  = "The date must be in the format: day-month-year (e.g., 01-05-2025)."
	Future_date_error                    = "The date you entered cannot be in the future. Please provide a valid past or present date."
	Date_conversion_error                = "There was an issue processing the date you provided. Please check and try again."
	Compound_check_error                 = "An error occurred while verifying the compound. Please try again shortly."
	Item_not_found                       = "The specified compound could not be found."
	Stock_retrieval_error                = "We are currently unable to retrieve stock information. Please try again later."
	Insufficient_stock                   = "There is not enough stock available for your request."
	Add_new_item_error                   = "A problem occurred while recording the quantity. Please try again."
	Save_entry_details_error             = "We were unable to save the details you entered. Please try again."
	Update_subsequent_entries_error      = "An issue arose while updating related stock information. Please try again."
	Record_transaction_error             = "We could not initiate the process to save this entry at this time. Please try again later."
	Commit_transaction_error             = "We were unable to finalize saving this entry. Please try again later."
	Entry_update_scan_error              = "An error occurred while reading updated stock information. Please try again later."
	Entry_inserted_successfully          = "Your entry has been successfully saved."
	Entry_updated_successfully           = "Entry updated successfully."
	Internal_server_error                = "A system error has occurred. Please try again later."
)
