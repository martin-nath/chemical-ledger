package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/utils"
	"github.com/sirupsen/logrus"
)

// TODO: fix the code structure of this file, make it similar to the insert-data.go file
// TODO: Reuse functions, if it can be
// TODO: Check all the responses and make sure they are correct in its situations

func UpdateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		utils.JsonRes(w, http.StatusMethodNotAllowed, &utils.Resp{
			Error: utils.InvalidMethod,
		})
		return
	}

	var entry utils.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.Req_body_decode_error,
		})
		logrus.Errorf("Error decoding request body: %v", err)
		return
	}

	logrus.Infof("Received request to update data: %+v", entry)

	if (entry.Type != "" && entry.Type != utils.TypeIncoming && entry.Type != utils.TypeOutgoing) || entry.ID == "" || entry.QuantityPerUnit < 0 || entry.NumOfUnits < 0 {
		utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
			Error: utils.MissingFields_or_inappropriate_value,
		})
		logrus.Warn("Missing or invalid required fields in the request.")
		return
	}

	var entryDate int64
	var err error
	if entry.Date != "" {
		entryDate, err = utils.UnixTimestamp(entry.Date)
		if err != nil {
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
				Error: utils.Invalid_date_format,
			})
			logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
			return
		}

		parsedDate, err := time.Parse("02-01-2006", entry.Date)
		if err != nil {
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
				Error: utils.Invalid_date_format,
			})
			logrus.Warnf("Invalid date format provided: %s, error: %v", entry.Date, err)
			return
		}

		if parsedDate.After(time.Now()) {
			utils.JsonRes(w, http.StatusBadRequest, &utils.Resp{
				Error: utils.Future_date_error,
			})
			logrus.Warnf("Future date provided: %s", entry.Date)
			return
		}
	}

	var dbTx *sql.Tx

	dbTx, err = db.Db.Begin()
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error starting transaction: %v", err)
		return
	}
	defer dbTx.Commit()

	if entryDate == 0 {
		if err = dbTx.QueryRow(`SELECT date FROM entry WHERE id = ?`, entry.ID).Scan(&entryDate); err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error retrieving entry date: %v", err)
			return
		}
	}

	var compoundIdValid bool
	if entry.CompoundId != "" {
		err = dbTx.QueryRow(`SELECT EXISTS(SELECT 1 FROM compound WHERE id = ?)`, entry.CompoundId).Scan(&compoundIdValid)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error checking if compound exists: %v", err)
			return
		}
	}

	var compoundId string
	err = dbTx.QueryRow(`SELECT compound_id FROM entry where id = ?`, entry.ID).Scan(&compoundId)
	if err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error retrieving old entry type: %v", err)
		return
	}

	var oldQuantityId string
	if entry.QuantityPerUnit != 0 || entry.NumOfUnits != 0 {
		err = dbTx.QueryRow(`SELECT quantity_id FROM entry where id = ?`, entry.ID).Scan(&oldQuantityId)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error retrieving old quantity id: %v", err)
			return
		}
	}

	updateQueryBuilder := strings.Builder{}

	switch {
	case entry.Type != "":
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE entry SET type = '%s' WHERE id = '%s';\n", entry.Type, entry.ID))
	case entry.Date != "":
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE entry SET date = '%d' WHERE id = '%s';\n", entryDate, entry.ID))
	case entry.QuantityPerUnit != 0:
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE quantity SET quantity_per_unit = '%d' WHERE id = '%s';\n", entry.QuantityPerUnit, oldQuantityId))
	case entry.NumOfUnits != 0:
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE quantity SET num_of_units = '%d' WHERE id = '%s';\n", entry.NumOfUnits, oldQuantityId))
	case entry.Remark != "":
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE entry SET remark = '%s' WHERE id = '%s';\n", entry.Remark, entry.ID))
	case entry.VoucherNo != "":
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE entry SET voucher_no = '%s' WHERE id = '%s';\n", entry.VoucherNo, entry.ID))
	case compoundIdValid:
		updateQueryBuilder.WriteString(fmt.Sprintf("UPDATE entry SET compound_id = '%s' WHERE id = '%s';\n", entry.CompoundId, entry.ID))
	}

	updateQuery := updateQueryBuilder.String()
	if updateQuery != "" {
		_, err = dbTx.Exec(updateQuery)
		if err != nil {
			utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
				Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
			})
			logrus.Errorf("Error updating entry: %v", err)
			return
		}
		logrus.Debugf("Updated entry with ID '%s'", entry.ID)
	}

	wg := sync.WaitGroup{}
	if compoundIdValid {
		wg.Add(1)
		go func(dbTx *sql.Tx, entryDate int64, newCompoundId string, w http.ResponseWriter) {
			defer wg.Done()
			utils.UpdateSubSequentNetStock(dbTx, entryDate, newCompoundId, w)
		}(dbTx, entryDate, entry.CompoundId, w)
	}

	utils.UpdateSubSequentNetStock(dbTx, entryDate, compoundId, w)

	wg.Wait()

	if err := dbTx.Commit(); err != nil {
		utils.JsonRes(w, http.StatusInternalServerError, &utils.Resp{
			Error: "Sorry, we're having trouble getting the stock information right now. Please try again later.",
		})
		logrus.Errorf("Error committing transaction: %v", err)
		return
	}

	logrus.Debug("Database transaction committed successfully.")

	utils.JsonRes(w, http.StatusOK, &utils.Resp{
		Message: "Entry updated successfully.",
		Data:    map[string]any{"entry_id": entry.ID},
	})
}
