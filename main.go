package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB
var jwtKey []byte

type App struct {
	Router *mux.Router
	DB     *sql.DB
}

type contextKey string
const userContextKey = contextKey("user")

type AuthenticatedUser struct {
	ID   string
	Type string
}

type FullExaminationRecord struct {
	ExaminationRecord
	PersonalData Patient `json:"personalData"`
}

func (a *App) Initialize() {
	err := godotenv.Load()
	if err != nil { log.Println("Perhatian: Tidak dapat memuat file .env.") }
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" { connStr = "postgres://prima_db:prima_db@db:5432/prima_db?sslmode=disable" }

	for i := 0; i < 5; i++ {
		a.DB, err = sql.Open("postgres", connStr)
		if err == nil {
			if err = a.DB.Ping(); err == nil { break }
		}
		log.Println("Gagal terhubung ke database, mencoba lagi dalam 2 detik...")
		time.Sleep(2 * time.Second)
	}
	if err != nil { log.Fatal("Tidak bisa terhubung ke database setelah beberapa kali percobaan:", err) }

	fmt.Println("Berhasil terhubung ke PostgreSQL!")
	jwtSecret := os.Getenv("JWT_SECRET_KEY")
	if jwtSecret == "" { jwtSecret = "kunci-rahasia-default-jangan-dipakai-di-produksi" }
	jwtKey = []byte(jwtSecret)
	a.Router = mux.NewRouter()
	a.initializeRoutes()
}

func (a *App) Run(addr string) {
	log.Printf("Server berjalan di port %s", addr)
	corsHandler := handlers.CORS(
		handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedOrigins([]string{"*"}),
	)
	log.Fatal(http.ListenAndServe(addr, corsHandler(a.Router)))
}

type Posyandu struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	AccountType  string    `json:"type"`
	PatientType  *string   `json:"patientType,omitempty"`
	Posyandu     *Posyandu `json:"posyandu"`
}

type Patient struct {
	ID          string    `json:"id"`
	FullName    string    `json:"fullName"`
	DateOfBirth time.Time `json:"dateOfBirth"`
	MotherName  string    `json:"motherName"`
	MotherPhone string    `json:"motherPhone"`
	PatientType string    `json:"patientType"`
}

type HemoglobinResult struct {
	AverageHb float64 `json:"averageHb"`
}

type ExaminationRecord struct {
	ID               string          `json:"id"`
	PatientID        string          `json:"patientId"`
	WeightHistory    json.RawMessage `json:"weightHistory"`
	HeightHistory    json.RawMessage `json:"heightHistory"`
	NutrientHistory  json.RawMessage `json:"nutrientHistory"`
	DenverMilestones json.RawMessage `json:"denverMilestones"`
	Age              string          `json:"age"`
	PatientType      string          `json:"patientType,omitempty"`
	ExaminationDate  time.Time       `json:"examinationDate"`
	TB               *float64        `json:"tb,omitempty"`
	BB               *float64        `json:"bb,omitempty"`
	Lila             *float64        `json:"lila,omitempty"`
	TbUZscore        *float64        `json:"tbU_zscore,omitempty"`
	BbUZscore        *float64        `json:"bbU_zscore,omitempty"`
	Imt              *float64        `json:"imt,omitempty"`
	IsTtdRutin       *bool           `json:"isTtdRutin,omitempty"`
	BbGainPerMonth   *float64        `json:"bbGainPerMonth,omitempty"`
	IsBbStagnan      *bool           `json:"isBbStagnan,omitempty"`
	HemoglobinResult json.RawMessage `json:"hemoglobinResult"`
	PmtHistory       json.RawMessage `json:"pmtHistory"`
}

type PmtItem struct {
	ID                 int    `json:"id,omitempty"`
	IconName           string `json:"iconName"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	StockCount         int    `json:"stockCount"`
	TargetGroup        string `json:"targetGroup"`
	SubItemTitle       string `json:"subItemTitle"`
	SubItemDescription string `json:"subItemDescription"`
}

type AttendanceRecord struct {
	Name              string                 `json:"name"`
	PatientType       string                 `json:"patientType"`
	Status            string                 `json:"status"`
	ExaminationRecord *FullExaminationRecord `json:"examinationRecord,omitempty"`
}

const ( RiskHigh = "High"; RiskMedium = "Medium"; RiskSafe = "Safe" )

func (rec *ExaminationRecord) CalculateRiskLevel() string {
	var hbResult HemoglobinResult
	if len(rec.HemoglobinResult) > 0 && rec.HemoglobinResult[0] != 'n' { json.Unmarshal(rec.HemoglobinResult, &hbResult) }
	switch rec.PatientType {
	case "child":
		c := 0; if rec.TbUZscore != nil && *rec.TbUZscore < -3 { c++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb < 10 { c++ }; if rec.Lila != nil && *rec.Lila < 11.5 { c++ }; if c >= 2 { return RiskHigh }
		m := 0; if rec.TbUZscore != nil && *rec.TbUZscore <= -2.0 { m++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb <= 10.9 { m++ }; if rec.Lila != nil && *rec.Lila <= 12.4 { m++ }; if m > 0 { return RiskMedium }
	case "pregnantWoman":
		c := 0; if rec.Lila != nil && *rec.Lila < 22 { c++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb < 10 { c++ }; if rec.IsBbStagnan != nil && *rec.IsBbStagnan { c++ }; if rec.IsTtdRutin != nil && !*rec.IsTtdRutin { c++ }; if c >= 2 { return RiskHigh }
		m := 0; if rec.Lila != nil && *rec.Lila <= 23.4 { m++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb <= 10.9 { m++ }; if rec.BbGainPerMonth != nil && *rec.BbGainPerMonth < 1 { m++ }; if rec.IsTtdRutin != nil && !*rec.IsTtdRutin { m++ }; if m > 0 { return RiskMedium }
	case "adolescentGirl":
		c := 0; if rec.Lila != nil && *rec.Lila < 22 { c++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb < 11 { c++ }; if rec.Imt != nil && *rec.Imt < 17 { c++ }; if rec.IsTtdRutin != nil && !*rec.IsTtdRutin { c++ }; if c >= 2 { return RiskHigh }
		m := 0; if rec.Lila != nil && *rec.Lila <= 23.4 { m++ }; if hbResult.AverageHb != 0 && hbResult.AverageHb <= 11.9 { m++ }; if rec.Imt != nil && *rec.Imt <= 18.4 { m++ }; if rec.IsTtdRutin != nil && !*rec.IsTtdRutin { m++ }; if m > 0 { return RiskMedium }
	}
	return RiskSafe
}

func (a *App) loginHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct { Username string `json:"username"`; Password string `json:"password"`}; if err := json.NewDecoder(r.Body).Decode(&creds); err != nil { respondWithError(w, http.StatusBadRequest, "Request body tidak valid"); return }
	user := User{Posyandu: &Posyandu{}}; var patientType sql.NullString
	err := a.DB.QueryRow(`SELECT u.id, u.name, u.username, u.password_hash, u.account_type, u.patient_type, p.name, p.address FROM users u JOIN posyandu p ON u.posyandu_id = p.id WHERE u.username = $1`, creds.Username).Scan(&user.ID, &user.Name, &user.Username, &user.PasswordHash, &user.AccountType, &patientType, &user.Posyandu.Name, &user.Posyandu.Address)
	if err != nil { if err == sql.ErrNoRows { respondWithError(w, http.StatusUnauthorized, "Username atau password salah") } else { respondWithError(w, http.StatusInternalServerError, err.Error()) }; return }
	if patientType.Valid { user.PatientType = &patientType.String }
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(creds.Password)); err != nil { respondWithError(w, http.StatusUnauthorized, "Username atau password salah"); return }
	claims := jwt.MapClaims{"sub": user.ID, "exp": time.Now().Add(24 * time.Hour).Unix(), "type": user.AccountType}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims); tokenString, err := token.SignedString(jwtKey); if err != nil { respondWithError(w, http.StatusInternalServerError, "Gagal membuat token"); return }
	respondWithJSON(w, http.StatusOK, map[string]interface{}{"token": tokenString, "user": user})
}

func (a *App) createPatientHandler(w http.ResponseWriter, r *http.Request) {
    var p Patient; if err := json.NewDecoder(r.Body).Decode(&p); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    err := a.DB.QueryRow(`INSERT INTO patients (id, full_name, date_of_birth, mother_name, mother_phone, patient_type) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, p.ID, p.FullName, p.DateOfBirth, p.MotherName, p.MotherPhone, p.PatientType).Scan(&p.ID)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; respondWithJSON(w, http.StatusCreated, p)
}

func (a *App) getAllPatientsHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := a.DB.Query("SELECT id, full_name, date_of_birth, mother_name, mother_phone, patient_type FROM patients ORDER BY full_name"); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
    patients := []Patient{}; for rows.Next() { var p Patient; if err := rows.Scan(&p.ID, &p.FullName, &p.DateOfBirth, &p.MotherName, &p.MotherPhone, &p.PatientType); err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; patients = append(patients, p) }; respondWithJSON(w, http.StatusOK, patients)
}

func (a *App) updatePatientHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]; var p Patient; if err := json.NewDecoder(r.Body).Decode(&p); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    _, err := a.DB.Exec(`UPDATE patients SET full_name=$1, date_of_birth=$2, mother_name=$3, mother_phone=$4, patient_type=$5 WHERE id=$6`, p.FullName, p.DateOfBirth, p.MotherName, p.MotherPhone, p.PatientType, id)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; p.ID = id; respondWithJSON(w, http.StatusOK, p)
}

func (a *App) deletePatientHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]; res, err := a.DB.Exec("DELETE FROM patients WHERE id=$1", id); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }
    count, _ := res.RowsAffected(); if count == 0 { respondWithError(w, http.StatusNotFound, "Patient not found"); return }; respondWithJSON(w, http.StatusOK, map[string]string{"message": "Patient deleted successfully"})
}

func (a *App) createExaminationRecordHandler(w http.ResponseWriter, r *http.Request) {
	var rec ExaminationRecord; if err := json.NewDecoder(r.Body).Decode(&rec); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
	query := `INSERT INTO examination_records (patient_id, age, examination_date, tb, bb, lila, tbu_zscore, bbu_zscore, imt, is_ttd_rutin, bb_gain_per_month, is_bb_stagnan, weight_history, height_history, nutrient_history, denver_milestones, hemoglobin_result, pmt_history) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18) RETURNING id;`
	err := a.DB.QueryRow(query, rec.PatientID, rec.Age, rec.ExaminationDate, rec.TB, rec.BB, rec.Lila, rec.TbUZscore, rec.BbUZscore, rec.Imt, rec.IsTtdRutin, rec.BbGainPerMonth, rec.IsBbStagnan, rec.WeightHistory, rec.HeightHistory, rec.NutrientHistory, rec.DenverMilestones, rec.HemoglobinResult, rec.PmtHistory).Scan(&rec.ID)
	if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; respondWithJSON(w, http.StatusCreated, rec)
}

func (a *App) deleteExaminationRecordHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]; res, err := a.DB.Exec("DELETE FROM examination_records WHERE id=$1", id); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }
	count, _ := res.RowsAffected(); if count == 0 { respondWithError(w, http.StatusNotFound, "Record not found"); return }; respondWithJSON(w, http.StatusOK, map[string]string{"message": "Examination record deleted"})
}

func (a *App) createPmtItemHandler(w http.ResponseWriter, r *http.Request) {
    var item PmtItem; if err := json.NewDecoder(r.Body).Decode(&item); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    query := `INSERT INTO pmt_items (icon_name, title, description, stock_count, target_group, sub_item_title, sub_item_description) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
    err := a.DB.QueryRow(query, item.IconName, item.Title, item.Description, item.StockCount, item.TargetGroup, item.SubItemTitle, item.SubItemDescription).Scan(&item.ID)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; respondWithJSON(w, http.StatusCreated, item)
}

func (a *App) updatePmtItemHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]; var item PmtItem; if err := json.NewDecoder(r.Body).Decode(&item); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    query := `UPDATE pmt_items SET icon_name=$1, title=$2, description=$3, stock_count=$4, target_group=$5, sub_item_title=$6, sub_item_description=$7 WHERE id=$8`
    _, err := a.DB.Exec(query, item.IconName, item.Title, item.Description, item.StockCount, item.TargetGroup, item.SubItemTitle, item.SubItemDescription, id)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; item.ID, _ = strconv.Atoi(id); respondWithJSON(w, http.StatusOK, item)
}

func (a *App) deletePmtItemHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]; res, err := a.DB.Exec("DELETE FROM pmt_items WHERE id=$1", id); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }
	count, _ := res.RowsAffected(); if count == 0 { respondWithError(w, http.StatusNotFound, "PMT item not found"); return }; respondWithJSON(w, http.StatusOK, map[string]string{"message": "PMT item deleted"})
}

func (a *App) createOrUpdateAttendanceHandler(w http.ResponseWriter, r *http.Request) {
    var payload struct { PatientID string `json:"patientId"`; Date string `json:"date"`; Status string `json:"status"`}
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    query := `INSERT INTO attendance (patient_id, date, status) VALUES ($1, $2, $3) ON CONFLICT (patient_id, date) DO UPDATE SET status = EXCLUDED.status`
    _, err := a.DB.Exec(query, payload.PatientID, payload.Date, payload.Status); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; respondWithJSON(w, http.StatusCreated, payload)
}

func (a *App) deleteAttendanceRecordHandler(w http.ResponseWriter, r *http.Request) {
    var payload struct { PatientID string `json:"patientId"`; Date string `json:"date"`}
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request body"); return }
    _, err := a.DB.Exec(`DELETE FROM attendance WHERE patient_id=$1 AND date=$2`, payload.PatientID, payload.Date); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; w.WriteHeader(http.StatusNoContent)
}

func (a *App) getLatestHealthRecordsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.rn = 1", "ORDER BY lr.examination_date DESC")); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
	records := scanFullExaminationRecords(rows); if records == nil { respondWithError(w, http.StatusInternalServerError, "Failed to scan records"); return }; respondWithJSON(w, http.StatusOK, records)
}

func (a *App) getPatientDetailsHandler(w http.ResponseWriter, r *http.Request) {
	patientID := mux.Vars(r)["patientId"]; rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.patient_id = $1 AND lr.rn = 1", ""), patientID); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
	records := scanFullExaminationRecords(rows); if len(records) == 0 { respondWithError(w, http.StatusNotFound, "Patient not found"); return }; respondWithJSON(w, http.StatusOK, records[0])
}

func (a *App) getLatestExaminationForMonthHandler(w http.ResponseWriter, r *http.Request) {
    patientID := mux.Vars(r)["patientId"]; month, _ := strconv.Atoi(r.URL.Query().Get("month")); year, _ := strconv.Atoi(r.URL.Query().Get("year"))
    rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.patient_id = $1 AND EXTRACT(MONTH FROM lr.examination_date) = $2 AND EXTRACT(YEAR FROM lr.examination_date) = $3", "ORDER BY lr.examination_date DESC LIMIT 1"), patientID, month, year)
	if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close(); records := scanFullExaminationRecords(rows)
	if len(records) == 0 { respondWithError(w, http.StatusNotFound, "No record found for this month"); return }; respondWithJSON(w, http.StatusOK, records[0])
}

func (a *App) getExaminationHistoryHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.rn = 1", "ORDER BY p.patient_type, lr.examination_date DESC")); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
    allRecords := scanFullExaminationRecords(rows)
    categorizedData := map[string]struct{ SummaryCount string `json:"summaryCount"`; Records []FullExaminationRecord `json:"records"`}{"Anak-anak": {Records: []FullExaminationRecord{}}, "Ibu Hamil": {Records: []FullExaminationRecord{}}, "Remaja Putri": {Records: []FullExaminationRecord{}}}
    for _, rec := range allRecords {
        switch rec.PersonalData.PatientType {
        case "child": cat := categorizedData["Anak-anak"]; cat.Records = append(cat.Records, rec); categorizedData["Anak-anak"] = cat
        case "pregnantWoman": cat := categorizedData["Ibu Hamil"]; cat.Records = append(cat.Records, rec); categorizedData["Ibu Hamil"] = cat
        case "adolescentGirl": cat := categorizedData["Remaja Putri"]; cat.Records = append(cat.Records, rec); categorizedData["Remaja Putri"] = cat
        }
    }
    anak := categorizedData["Anak-anak"]; anak.SummaryCount = strconv.Itoa(len(anak.Records)); categorizedData["Anak-anak"] = anak
    hamil := categorizedData["Ibu Hamil"]; hamil.SummaryCount = strconv.Itoa(len(hamil.Records)); categorizedData["Ibu Hamil"] = hamil
    remaja := categorizedData["Remaja Putri"]; remaja.SummaryCount = strconv.Itoa(len(remaja.Records)); categorizedData["Remaja Putri"] = remaja
    respondWithJSON(w, http.StatusOK, categorizedData)
}

func (a *App) updateExaminationRecordHandler(w http.ResponseWriter, r *http.Request) {
	patientID := mux.Vars(r)["patientId"]; var payload struct { HemoglobinResult json.RawMessage `json:"hemoglobinResult"`; BB *float64 `json:"bb"`; TB *float64 `json:"tb"`; Lila *float64 `json:"lila"` }
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request payload"); return }
	query := `WITH latest_record AS (SELECT id FROM examination_records WHERE patient_id = $5 ORDER BY examination_date DESC LIMIT 1) UPDATE examination_records SET hemoglobin_result = COALESCE($1, hemoglobin_result), bb = COALESCE($2, bb), tb = COALESCE($3, tb), lila = COALESCE($4, lila) WHERE id = (SELECT id FROM latest_record) RETURNING id;`
	var updatedID string; err := a.DB.QueryRow(query, payload.HemoglobinResult, payload.BB, payload.TB, payload.Lila, patientID).Scan(&updatedID)
	if err != nil { respondWithError(w, http.StatusInternalServerError, "Failed to update record: "+err.Error()); return }
	rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.id = $1", ""), updatedID); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
	records := scanFullExaminationRecords(rows); if len(records) == 0 { respondWithError(w, http.StatusNotFound, "Updated record not found"); return }; respondWithJSON(w, http.StatusOK, records[0])
}

func (a *App) getPmtItemsHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := a.DB.Query("SELECT id, icon_name, title, description, stock_count, target_group, sub_item_title, sub_item_description FROM pmt_items"); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
    items := []PmtItem{}; for rows.Next() { var item PmtItem; if err := rows.Scan(&item.ID, &item.IconName, &item.Title, &item.Description, &item.StockCount, &item.TargetGroup, &item.SubItemTitle, &item.SubItemDescription); err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; items = append(items, item) }; respondWithJSON(w, http.StatusOK, items)
}

func (a *App) getDashboardDataHandler(w http.ResponseWriter, r *http.Request) {
    rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.rn = 1", "")); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
    allLatestRecords := scanFullExaminationRecords(rows); highRiskChildren, highRiskMothers, highRiskAdolescents := 0, 0, 0
    for _, rec := range allLatestRecords { rec.ExaminationRecord.PatientType = rec.PersonalData.PatientType; if rec.CalculateRiskLevel() == RiskHigh { switch rec.PersonalData.PatientType { case "child": highRiskChildren++; case "pregnantWoman": highRiskMothers++; case "adolescentGirl": highRiskAdolescents++ } } }
    chartData, err := a.getChartData(); if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }
    response := map[string]interface{}{
        "riskSummary": map[string]string{ "childRisk": fmt.Sprintf("%d Children at risk of Stunting & Anemia", highRiskChildren), "pregnantRisk":   fmt.Sprintf("%d Mothers at risk of High LBW", highRiskMothers), "adolescentRisk": fmt.Sprintf("%d Adolescents at risk of Anemia", highRiskAdolescents), },
        "chartData": chartData,
    }; respondWithJSON(w, http.StatusOK, response)
}

func (a *App) getChartData() (map[string]interface{}, error) {
    twelveMonthsAgo := time.Now().AddDate(0, -11, 0)
    query := `SELECT p.patient_type, date_trunc('month', er.examination_date) as month, er.lila, er.tbu_zscore, er.is_bb_stagnan, er.is_ttd_rutin, er.imt, er.hemoglobin_result FROM examination_records er JOIN patients p ON er.patient_id = p.id WHERE er.examination_date >= $1`
    rows, err := a.DB.Query(query, twelveMonthsAgo); if err != nil { return nil, err }; defer rows.Close()
    type monthlyStat struct { High, Medium, Safe int }
    stats := make(map[string]map[time.Time]monthlyStat)
    for rows.Next() {
        var rec ExaminationRecord; var month time.Time
        var lila, tbuz, imt sql.NullFloat64; var stagnan, ttdrutin sql.NullBool; var hemoglobin sql.NullString
        if err := rows.Scan(&rec.PatientType, &month, &lila, &tbuz, &stagnan, &ttdrutin, &imt, &hemoglobin); err != nil { return nil, err }
        if lila.Valid { rec.Lila = &lila.Float64 }; if tbuz.Valid { rec.TbUZscore = &tbuz.Float64 }; if imt.Valid { rec.Imt = &imt.Float64 }
        if stagnan.Valid { rec.IsBbStagnan = &stagnan.Bool }; if ttdrutin.Valid { rec.IsTtdRutin = &ttdrutin.Bool }
        if hemoglobin.Valid { rec.HemoglobinResult = json.RawMessage(hemoglobin.String) }

        if _, ok := stats[rec.PatientType]; !ok { stats[rec.PatientType] = make(map[time.Time]monthlyStat) }
        stat := stats[rec.PatientType][month]
        risk := rec.CalculateRiskLevel()
        if risk == RiskHigh { stat.High++ } else if risk == RiskMedium { stat.Medium++ } else { stat.Safe++ }
        stats[rec.PatientType][month] = stat
    }
    type flSpot struct { X float64 `json:"x"`; Y float64 `json:"y"` }
    buildSpots := func(patientType string) map[string][]flSpot {
        high, medium, safe := []flSpot{}, []flSpot{}, []flSpot{}
        now := time.Now()
        for i := 0; i < 12; i++ {
            month := now.AddDate(0, -i, 0)
            monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
            x := float64(12 - i)
            var s monthlyStat
            if monthStats, ok := stats[patientType]; ok { s = monthStats[monthStart] }
            high = append(high, flSpot{X: x, Y: float64(s.High)})
            medium = append(medium, flSpot{X: x, Y: float64(s.Medium)})
            safe = append(safe, flSpot{X: x, Y: float64(s.Safe)})
        }
        return map[string][]flSpot{"highRisk": high, "mediumRisk": medium, "lowRisk": safe}
    }
    return map[string]interface{}{"child": buildSpots("child"), "pregnantWoman": buildSpots("pregnantWoman"), "adolescentGirl": buildSpots("adolescentGirl")}, nil
}

func (a *App) getTodaysAttendanceHandler(w http.ResponseWriter, r *http.Request) {
    today := time.Now().Format("2006-01-02"); query := `SELECT p.full_name, p.patient_type, a.status FROM attendance a JOIN patients p ON a.patient_id = p.id WHERE a.date = $1`; rows, err := a.DB.Query(query, today)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close(); records := []AttendanceRecord{}
    for rows.Next() { var rec AttendanceRecord; if err := rows.Scan(&rec.Name, &rec.PatientType, &rec.Status); err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; records = append(records, rec) }; respondWithJSON(w, http.StatusOK, records)
}

func (a *App) getAttendanceRecordsHandler(w http.ResponseWriter, r *http.Request) {
    date := r.URL.Query().Get("date"); category := r.URL.Query().Get("category"); rows, err := a.DB.Query(`SELECT p.full_name, p.patient_type, a.status FROM attendance a JOIN patients p ON a.patient_id = p.id WHERE a.date = $1 AND p.patient_type = $2`, date, category)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close(); records := []AttendanceRecord{}
    for rows.Next() { var rec AttendanceRecord; if err := rows.Scan(&rec.Name, &rec.PatientType, &rec.Status); err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; records = append(records, rec) }; respondWithJSON(w, http.StatusOK, records)
}

func generateInterventionAnalysis(rec FullExaminationRecord) string {
    var hbResult HemoglobinResult
    if len(rec.HemoglobinResult) > 0 && rec.HemoglobinResult[0] != 'n' { json.Unmarshal(rec.HemoglobinResult, &hbResult) }
    name := rec.PersonalData.FullName
    switch rec.PersonalData.PatientType {
    case "child":
        tbU := "N/A"; if rec.TbUZscore != nil { tbU = fmt.Sprintf("%.1f", *rec.TbUZscore) }; hb := "N/A"; if hbResult.AverageHb != 0 { hb = fmt.Sprintf("%.1f", hbResult.AverageHb) }
        return fmt.Sprintf("Pasien ini, %s, menunjukkan tanda-tanda darurat gizi yaitu stunting berat (TB/U: %s) dan kemungkinan anemia (Hb: %s), yang sangat berisiko menghambat pertumbuhan dan perkembangan kognitifnya. Prioritas utama kader adalah segera memberikan rujukan ke Puskesmas untuk penanganan medis. Sambil menunggu, jadwalkan kunjungan rumah untuk mengedukasi ibu tentang pentingnya MPASI kaya zat besi seperti hati ayam.", name, tbU, hb)
    case "pregnantWoman":
        lila := "N/A"; if rec.Lila != nil { lila = fmt.Sprintf("%.1f", *rec.Lila) }
        return fmt.Sprintf("Ibu %s teridentifikasi memiliki risiko tinggi Kekurangan Energi Kronis (KEK) berdasarkan ukuran LiLA (%s cm) dan kenaikan berat badan yang stagnan. Kondisi ini dapat berdampak pada berat badan lahir rendah (BBLR) pada bayi. Intervensi paling mendesak adalah memberikan PMT Pemulihan khusus untuk ibu hamil dan melakukan kunjungan rumah untuk konseling gizi intensif.", name, lila)
    case "adolescentGirl":
        imt := "N/A"; if rec.Imt != nil { imt = fmt.Sprintf("%.1f", *rec.Imt) }
        return fmt.Sprintf("Remaja putri %s menunjukkan status gizi sangat kurus (IMT: %s) dan kemungkinan mengalami anemia ringan. Kondisi ini perlu segera ditangani untuk mencegah masalah kesehatan jangka panjang. Rekomendasi utama adalah memberikan edukasi gizi seimbang dan memastikan kepatuhan konsumsi TTD mingguan.", name, imt)
    }
    return fmt.Sprintf("Analisis umum untuk %s: Mohon periksa data untuk rekomendasi lebih lanjut.", name)
}

func (a *App) analysisHandler(w http.ResponseWriter, r *http.Request) {
    var rec FullExaminationRecord; if err := json.NewDecoder(r.Body).Decode(&rec); err != nil { respondWithError(w, http.StatusBadRequest, "Invalid request payload"); return }
    analysisText := generateInterventionAnalysis(rec); respondWithJSON(w, http.StatusOK, map[string]string{"analysisText": analysisText})
}

func (a *App) getVulnerablePatientsHandler(w http.ResponseWriter, r *http.Request) {
    category := r.URL.Query().Get("category"); rows, err := a.DB.Query(getFullExaminationRecordQuery("lr.rn = 1 AND p.patient_type = $1", "ORDER BY p.full_name"), category)
    if err != nil { respondWithError(w, http.StatusInternalServerError, err.Error()); return }; defer rows.Close()
    allRecords := scanFullExaminationRecords(rows); vulnerableRecords := []FullExaminationRecord{}
    for _, rec := range allRecords { rec.ExaminationRecord.PatientType = rec.PersonalData.PatientType; if rec.CalculateRiskLevel() == RiskHigh { vulnerableRecords = append(vulnerableRecords, rec) } }; respondWithJSON(w, http.StatusOK, vulnerableRecords)
}

func (a *App) initializeRoutes() {
    a.Router.Use(loggingMiddleware); apiV1 := a.Router.PathPrefix("/api/v1").Subrouter(); apiV1.HandleFunc("/auth/login", a.loginHandler).Methods("POST"); authRoutes := apiV1.PathPrefix("").Subrouter(); authRoutes.Use(a.jwtAuthenticationMiddleware)
    authRoutes.HandleFunc("/dashboard", a.getDashboardDataHandler).Methods("GET"); authRoutes.HandleFunc("/patients/vulnerable", a.getVulnerablePatientsHandler).Methods("GET"); authRoutes.HandleFunc("/analysis/intervention", a.analysisHandler).Methods("POST")
    authRoutes.HandleFunc("/patients", a.createPatientHandler).Methods("POST"); authRoutes.HandleFunc("/patients", a.getAllPatientsHandler).Methods("GET"); authRoutes.HandleFunc("/patients/{id}", a.updatePatientHandler).Methods("PUT"); authRoutes.HandleFunc("/patients/{id}", a.deletePatientHandler).Methods("DELETE"); authRoutes.HandleFunc("/patients/{patientId}/details", a.getPatientDetailsHandler).Methods("GET"); authRoutes.HandleFunc("/patients/{patientId}/examinations/latest", a.getLatestExaminationForMonthHandler).Methods("GET")
    authRoutes.HandleFunc("/examinations", a.createExaminationRecordHandler).Methods("POST"); authRoutes.HandleFunc("/examinations/latest", a.getLatestHealthRecordsHandler).Methods("GET"); authRoutes.HandleFunc("/examinations/history", a.getExaminationHistoryHandler).Methods("GET"); authRoutes.HandleFunc("/examinations/{patientId}", a.updateExaminationRecordHandler).Methods("PUT"); authRoutes.HandleFunc("/examinations/{id}", a.deleteExaminationRecordHandler).Methods("DELETE")
    authRoutes.HandleFunc("/pmt/items", a.getPmtItemsHandler).Methods("GET"); authRoutes.HandleFunc("/pmt/items", a.createPmtItemHandler).Methods("POST"); authRoutes.HandleFunc("/pmt/items/{id}", a.updatePmtItemHandler).Methods("PUT"); authRoutes.HandleFunc("/pmt/items/{id}", a.deletePmtItemHandler).Methods("DELETE")
    authRoutes.HandleFunc("/attendance", a.createOrUpdateAttendanceHandler).Methods("POST"); authRoutes.HandleFunc("/attendance/today", a.getTodaysAttendanceHandler).Methods("GET"); authRoutes.HandleFunc("/attendance", a.getAttendanceRecordsHandler).Methods("GET"); authRoutes.HandleFunc("/attendance", a.deleteAttendanceRecordHandler).Methods("DELETE")
    a.Router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { respondWithJSON(w, http.StatusOK, map[string]string{"status": "Prima API Aktif ✅"}) })
}

func loggingMiddleware(next http.Handler) http.Handler { return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { log.Printf("Request: %s %s", r.Method, r.URL.Path); next.ServeHTTP(w, r) }) }
func (a *App) jwtAuthenticationMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization"); if authHeader == "" { respondWithError(w, http.StatusUnauthorized, "Authorization header required"); return }
        tokenString := strings.TrimPrefix(authHeader, "Bearer "); if tokenString == authHeader { respondWithError(w, http.StatusUnauthorized, "Invalid token format"); return }
        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) { if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok { return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"]) }; return jwtKey, nil })
        if err != nil || !token.Valid { respondWithError(w, http.StatusUnauthorized, "Invalid token"); return }
        claims, ok := token.Claims.(jwt.MapClaims); if !ok || !token.Valid { respondWithError(w, http.StatusUnauthorized, "Invalid token claims"); return }
        user := AuthenticatedUser{ID: claims["sub"].(string), Type: claims["type"].(string)}; ctx := context.WithValue(r.Context(), userContextKey, user); next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) { response, _ := json.Marshal(payload); w.Header().Set("Content-Type", "application/json"); w.WriteHeader(code); w.Write(response) }
func respondWithError(w http.ResponseWriter, code int, message string) { respondWithJSON(w, code, map[string]string{"error": message}) }

func getFullExaminationRecordQuery(whereClause, orderByClause string) string {
    baseQuery := `WITH latest_records AS (SELECT *, ROW_NUMBER() OVER(PARTITION BY patient_id ORDER BY examination_date DESC) as rn FROM examination_records) SELECT lr.id, p.id, p.full_name, p.date_of_birth, p.mother_name, p.mother_phone, lr.weight_history, lr.height_history, lr.nutrient_history, lr.denver_milestones, lr.age, p.patient_type, lr.examination_date, lr.tb, lr.bb, lr.lila, lr.tbu_zscore, lr.bbu_zscore, lr.imt, lr.is_ttd_rutin, lr.bb_gain_per_month, lr.is_bb_stagnan, lr.hemoglobin_result, lr.pmt_history FROM latest_records lr JOIN patients p ON lr.patient_id = p.id`
    if whereClause != "" { baseQuery += " WHERE " + whereClause }; if orderByClause != "" { baseQuery += " " + orderByClause }; return baseQuery + ";"
}

func scanFullExaminationRecords(rows *sql.Rows) []FullExaminationRecord {
    records := []FullExaminationRecord{}
    for rows.Next() {
        var rec FullExaminationRecord
        var weightHistory, heightHistory, nutrientHistory, denverMilestones, hemoglobinResult, pmtHistory sql.NullString

        err := rows.Scan(
            &rec.ID, 
            &rec.PersonalData.ID, 
            &rec.PersonalData.FullName, 
            &rec.PersonalData.DateOfBirth, 
            &rec.PersonalData.MotherName, 
            &rec.PersonalData.MotherPhone, 
            &weightHistory,        
            &heightHistory,        
            &nutrientHistory,   
            &denverMilestones,     
            &rec.Age, 
            &rec.PersonalData.PatientType, 
            &rec.ExaminationDate, 
            &rec.TB, 
            &rec.BB, 
            &rec.Lila, 
            &rec.TbUZscore, 
            &rec.BbUZscore, 
            &rec.Imt, 
            &rec.IsTtdRutin, 
            &rec.BbGainPerMonth, 
            &rec.IsBbStagnan, 
            &hemoglobinResult,      
            &pmtHistory,          
        )
        if err != nil {
            log.Printf("Error scanning record: %v", err)
            return nil 
        }

        if weightHistory.Valid { 
            rec.WeightHistory = []byte(weightHistory.String) 
        }
        if heightHistory.Valid { 
            rec.HeightHistory = []byte(heightHistory.String) 
        }
        if nutrientHistory.Valid { 
            rec.NutrientHistory = []byte(nutrientHistory.String) 
        }
        if denverMilestones.Valid { 
            rec.DenverMilestones = []byte(denverMilestones.String) 
        }
        if hemoglobinResult.Valid { 
            rec.HemoglobinResult = []byte(hemoglobinResult.String) 
        }
        if pmtHistory.Valid { 
            rec.PmtHistory = []byte(pmtHistory.String) 
        }

        rec.PatientID = rec.PersonalData.ID
        rec.PatientType = rec.PersonalData.PatientType
        records = append(records, rec)
    }
    return records
}

func createTables(db *sql.DB) {
    fmt.Println("Memulai migrasi database..."); enumQueries := `DO $$ BEGIN CREATE TYPE account_type AS ENUM ('kader', 'patient'); EXCEPTION WHEN duplicate_object THEN null; END $$; DO $$ BEGIN CREATE TYPE patient_type AS ENUM ('child', 'pregnantWoman', 'adolescentGirl'); EXCEPTION WHEN duplicate_object THEN null; END $$; DO $$ BEGIN CREATE TYPE attendance_status AS ENUM ('Present', 'Absent', 'Waiting'); EXCEPTION WHEN duplicate_object THEN null; END $$;`
    if _, err := db.Exec(enumQueries); err != nil { log.Fatalf("Gagal membuat tipe ENUM: %v", err) }
    tableQueries := `CREATE TABLE IF NOT EXISTS posyandu (id SERIAL PRIMARY KEY, name VARCHAR(100) NOT NULL, address VARCHAR(255)); CREATE TABLE IF NOT EXISTS users (id VARCHAR(10) PRIMARY KEY, name VARCHAR(100) NOT NULL, username VARCHAR(50) UNIQUE NOT NULL, password_hash VARCHAR(100) NOT NULL, account_type account_type NOT NULL, patient_type patient_type, posyandu_id INTEGER REFERENCES posyandu(id)); CREATE TABLE IF NOT EXISTS patients (id VARCHAR(10) PRIMARY KEY, full_name VARCHAR(100) NOT NULL, date_of_birth DATE NOT NULL, mother_name VARCHAR(100), mother_phone VARCHAR(20), patient_type patient_type NOT NULL); CREATE TABLE IF NOT EXISTS examination_records (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), patient_id VARCHAR(10) REFERENCES patients(id) ON DELETE CASCADE NOT NULL, age VARCHAR(50), examination_date TIMESTAMPTZ NOT NULL, tb NUMERIC(5, 2), bb NUMERIC(5, 2), lila NUMERIC(5, 2), tbu_zscore NUMERIC(5, 2), bbu_zscore NUMERIC(5, 2), imt NUMERIC(5, 2), is_ttd_rutin BOOLEAN, bb_gain_per_month NUMERIC(5, 2), is_bb_stagnan BOOLEAN, weight_history JSONB, height_history JSONB, nutrient_history JSONB, denver_milestones JSONB, hemoglobin_result JSONB, pmt_history JSONB, created_at TIMESTAMPTZ DEFAULT NOW()); CREATE TABLE IF NOT EXISTS pmt_items (id SERIAL PRIMARY KEY, icon_name VARCHAR(50), title VARCHAR(100) UNIQUE NOT NULL, description TEXT, stock_count INTEGER DEFAULT 0, target_group patient_type, sub_item_title VARCHAR(100), sub_item_description TEXT); CREATE TABLE IF NOT EXISTS attendance (id SERIAL PRIMARY KEY, patient_id VARCHAR(10) REFERENCES patients(id) ON DELETE CASCADE NOT NULL, date DATE NOT NULL, status attendance_status NOT NULL, UNIQUE(patient_id, date));`
    if _, err := db.Exec(tableQueries); err != nil { log.Fatalf("Gagal membuat tabel: %v", err) }; fmt.Println("Migrasi database selesai.")
}

func seedData(db *sql.DB) {
    var count int
    db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
    if count > 0 {
        fmt.Println("Data dummy sudah ada, seeding dilewati.")
        return
    }
    fmt.Println("Memulai proses seeding data dummy...")
    tx, err := db.Begin()
    if err != nil {
        log.Fatal("Gagal memulai transaksi: ", err)
    }

    var posyanduID int
    err = tx.QueryRow(`INSERT INTO posyandu (name, address) VALUES ($1, $2) RETURNING id`, "Posyandu Sehat Ceria", "Jl. Mawar No. 12, Jawa Barat").Scan(&posyanduID)
    if err != nil { tx.Rollback(); log.Fatal(err) }

    hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("12345"), bcrypt.DefaultCost)
    _, err = tx.Exec(`INSERT INTO users (id, name, username, password_hash, account_type, posyandu_id) VALUES ($1, $2, $3, $4, $5, $6)`, "KDR001", "Anisa (Bidan)", "kader", string(hashedPassword), "kader", posyanduID)
    if err != nil { tx.Rollback(); log.Fatal(err) }

    patientQuery := `INSERT INTO patients (id, full_name, date_of_birth, mother_name, mother_phone, patient_type) VALUES 
        ('NS001', 'Naufal Sabitululum', '2025-05-01', 'Siti Rahma', '081234567890', 'child'), 
        ('AK002', 'Adinda Kirana', '2025-01-10', 'Dewi Lestari', '089876543210', 'child'), 
        ('BS003', 'Budi Santoso', '2024-08-05', 'Rina Wati', '081122334455', 'child'),
        ('FA009', 'Fatimah Azzahra', '2024-12-15', 'Ani Suryani', '081311223344', 'child'),
        ('RP010', 'Rizky Pratama', '2025-02-20', 'Yuni Kartika', '081422334455', 'child'),
        ('SA004', 'Siti Aisyah', '1998-04-20', 'Siti Aisyah', '082233445566', 'pregnantWoman'), 
        ('RS005', 'Riana Sari', '1997-07-15', 'Riana Sari', '083344556677', 'pregnantWoman'), 
        ('DA008', 'Dewi Anggraini', '1996-11-12', 'Dewi Anggraini', '081298765432', 'pregnantWoman'),
        ('LI011', 'Lestari Indah', '1999-01-05', 'Lestari Indah', '081533445566', 'pregnantWoman'),
        ('MW012', 'Mega Wati', '1995-06-25', 'Mega Wati', '081644556677', 'pregnantWoman'),
        ('PA006', 'Putri Ayu', '2010-01-30', 'Lina Marlina', '085566778899', 'adolescentGirl'), 
        ('SA007', 'Siti Aminah', '2009-05-22', 'Nur Hasanah', '087788990011', 'adolescentGirl'),
        ('CC013', 'Cindy Claudia', '2008-09-10', 'Rina Marlina', '081755667788', 'adolescentGirl'),
        ('EY014', 'Eka Yulianti', '2009-03-18', 'Sri Wahyuni', '081866778899', 'adolescentGirl');`
    _, err = tx.Exec(patientQuery)
    if err != nil { tx.Rollback(); log.Fatal(err) }

    // -- Kategori: Anak --
    // Naufal Sabitululum (NS001) - Risiko Tinggi (Stunting & Anemia)
    whJSON_NS001 := `[{"recordedAtAgeMonth": 3, "value": 6.0}, {"recordedAtAgeMonth": 4, "value": 6.7}, {"recordedAtAgeMonth": 5, "value": 7.2}]`
    hhJSON_NS001 := `[{"recordedAtAgeMonth": 3, "value": 61}, {"recordedAtAgeMonth": 4, "value": 64}, {"recordedAtAgeMonth": 5, "value": 66}]`
    hbJSON_NS001 := `{"nailBedResults": [{"objectType": "nail", "confidence": 0.95, "hbValue": 9.8}], "conjunctivaResults": [], "averageHb": 9.8, "indication": "Anemia Berat", "confidenceLevel": 98.0, "nailBedIndication": "Anemia Berat"}`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, bbu_zscore, tbu_zscore, weight_history, height_history, hemoglobin_result) VALUES ('NS001', '5 Bulan', '2025-10-05', 7.2, 66, 11.2, -2.5, -3.1, $1, $2, $3);`, whJSON_NS001, hhJSON_NS001, hbJSON_NS001); if err != nil { tx.Rollback(); log.Fatal(err) }
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, hemoglobin_result) VALUES ('NS001', '4 Bulan', '2025-09-05', 6.7, 64, 11.0, '{"averageHb": 10.1, "indication": "Anemia Sedang"}'), ('NS001', '3 Bulan', '2025-08-05', 6.0, 61, 10.8, null);`); if err != nil { tx.Rollback(); log.Fatal(err) }

    // Adinda Kirana (AK002) - Risiko Sedang (TB/U mendekati -2)
    hbJSON_AK002 := `{"nailBedResults": [{"objectType": "nail", "confidence": 0.92, "hbValue": 11.2}], "conjunctivaResults": [{"objectType": "conjunctiva", "confidence": 0.90, "hbValue": 11.4}], "averageHb": 11.3, "indication": "Normal", "confidenceLevel": 91.0}`
    whJSON_AK002 := `[{"recordedAtAgeMonth": 7, "value": 7.8}, {"recordedAtAgeMonth": 8, "value": 8.2}, {"recordedAtAgeMonth": 9, "value": 8.5}]`
    hhJSON_AK002 := `[{"recordedAtAgeMonth": 7, "value": 68}, {"recordedAtAgeMonth": 8, "value": 70}, {"recordedAtAgeMonth": 9, "value": 72}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, tbu_zscore, weight_history, height_history, hemoglobin_result) VALUES ('AK002', '9 Bulan', '2025-10-11', 8.5, 72, 12.5, -1.8, $1, $2, $3);`, whJSON_AK002, hhJSON_AK002, hbJSON_AK002); if err != nil { tx.Rollback(); log.Fatal(err) }

    // Budi Santoso (BS003) - Aman
    whJSON_BS003 := `[{"recordedAtAgeMonth": 12, "value": 9.8}, {"recordedAtAgeMonth": 13, "value": 10.2}, {"recordedAtAgeMonth": 14, "value": 10.5}]`
    hhJSON_BS003 := `[{"recordedAtAgeMonth": 12, "value": 76}, {"recordedAtAgeMonth": 13, "value": 78}, {"recordedAtAgeMonth": 14, "value": 80}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, tbu_zscore, bbu_zscore, weight_history, height_history, hemoglobin_result) VALUES ('BS003', '1.2 Tahun', '2025-10-11', 10.5, 80, 13.0, -0.2, -0.5, $1, $2, '{"averageHb": 11.5}');`, whJSON_BS003, hhJSON_BS003); if err != nil { tx.Rollback(); log.Fatal(err) }

    // Fatimah Azzahra (FA009) - Aman, data bulan lalu
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, tbu_zscore, bbu_zscore, hemoglobin_result) VALUES ('FA009', '9 Bulan', '2025-09-15', 8.8, 73, 13.5, 0.1, 0.3, '{"averageHb": 12.0}');`); if err != nil { tx.Rollback(); log.Fatal(err) }

    // Rizky Pratama (RP010) - Risiko Sedang dengan data Denver & Nutrisi
    denverJSON_RP010 := `[{"task":"Duduk tanpa pegangan", "category":"grossMotor", "status":"pass"}, {"task":"Mengoceh (ma-ma, ba-ba)", "category":"language", "status":"pass"}, {"task":"Melambaikan tangan", "category":"personalSocial", "status":"borderline"}]`
    nutrientJSON_RP010 := `[{"name":"Vitamin A", "schedule":"Feb & Agu", "consumptionHistory":{"2025":[0,1,0,0,0,0,0,1,0,0,0,0]}}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, bbu_zscore, tbu_zscore, hemoglobin_result, denver_milestones, nutrient_history) VALUES ('RP010', '8 Bulan', '2025-10-12', 7.5, 69, 12.0, -1.9, -2.1, '{"averageHb": 10.8}', $1, $2);`, denverJSON_RP010, nutrientJSON_RP010); if err != nil { tx.Rollback(); log.Fatal(err) }

    // -- Kategori: Ibu Hamil --
    // Siti Aisyah (SA004) - Risiko Tinggi (KEK, Anemia, BB Stagnan)
    whJSON_SA004 := `[{"recordedAtAgeMonth": 5, "value": 55}, {"recordedAtAgeMonth": 6, "value": 55.8}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, is_ttd_rutin, is_bb_stagnan, bb_gain_per_month, weight_history, hemoglobin_result) VALUES ('SA004', '26 minggu', '2025-10-10', 55.8, 155, 21.4, false, true, 0.8, $1, '{"averageHb": 9.6}');`, whJSON_SA004); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Riana Sari (RS005) - Aman
    whJSON_RS005 := `[{"recordedAtAgeMonth": 3, "value": 51}, {"recordedAtAgeMonth": 4, "value": 52}, {"recordedAtAgeMonth": 5, "value": 53.2}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, is_ttd_rutin, is_bb_stagnan, bb_gain_per_month, weight_history, hemoglobin_result) VALUES ('RS005', '20 Minggu', '2025-10-01', 53.2, 160, 24.0, true, false, 1.2, $1, '{"averageHb": 11.5}');`, whJSON_RS005); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Dewi Anggraini (DA008) - Aman
    whJSON_DA008 := `[{"recordedAtAgeMonth": 5, "value": 58}, {"recordedAtAgeMonth": 6, "value": 60}, {"recordedAtAgeMonth": 7, "value": 61.5}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, is_ttd_rutin, is_bb_stagnan, bb_gain_per_month, weight_history, hemoglobin_result) VALUES ('DA008', '30 Minggu', '2025-10-05', 61.5, 158, 25.5, true, false, 1.5, $1, '{"averageHb": 12.5}');`, whJSON_DA008); if err != nil { tx.Rollback(); log.Fatal(err) }

    // Lestari Indah (LI011) - Risiko Sedang (LiLA batas, TTD tdk rutin)
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, is_ttd_rutin, bb_gain_per_month, hemoglobin_result) VALUES ('LI011', '24 Minggu', '2025-10-12', 58.0, 159, 23.4, false, 1.0, '{"averageHb": 11.1}');`); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Mega Wati (MW012) - Aman
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, bb, tb, lila, is_ttd_rutin, bb_gain_per_month, hemoglobin_result) VALUES ('MW012', '28 Minggu', '2025-09-28', 65.2, 162, 26.1, true, 1.8, '{"averageHb": 12.8}');`); if err != nil { tx.Rollback(); log.Fatal(err) }

    // -- Kategori: Remaja Putri --
    // Putri Ayu (PA006) - Risiko Sedang (IMT kurang, TTD tidak rutin)
    whJSON_PA006 := `[{"recordedAtAgeMonth": 175, "value": 38}, {"recordedAtAgeMonth": 188, "value": 40}]`
    hhJSON_PA006 := `[{"recordedAtAgeMonth": 175, "value": 152}, {"recordedAtAgeMonth": 188, "value": 155}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, tb, bb, lila, imt, is_ttd_rutin, weight_history, height_history, hemoglobin_result) VALUES ('PA006', '15.7 Tahun', '2025-08-30', 155, 40, 22.5, 16.6, false, $1, $2, '{"averageHb": 11.5}');`, whJSON_PA006, hhJSON_PA006); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Siti Aminah (SA007) - Aman
    whJSON_SA007 := `[{"recordedAtAgeMonth": 180, "value": 50}, {"recordedAtAgeMonth": 197, "value": 53}]`
    hhJSON_SA007 := `[{"recordedAtAgeMonth": 180, "value": 160}, {"recordedAtAgeMonth": 197, "value": 162}]`
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, tb, bb, lila, imt, is_ttd_rutin, weight_history, height_history, hemoglobin_result) VALUES ('SA007', '16.4 Tahun', '2025-10-08', 162, 53, 24.0, 20.2, true, $1, $2, '{"averageHb": 12.2}');`, whJSON_SA007, hhJSON_SA007); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Cindy Claudia (CC013) - Risiko Tinggi (IMT sangat kurang & Anemia)
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, tb, bb, lila, imt, is_ttd_rutin, hemoglobin_result) VALUES ('CC013', '17.1 Tahun', '2025-10-12', 158, 41, 21.8, 16.4, false, '{"averageHb": 10.5}');`); if err != nil { tx.Rollback(); log.Fatal(err) }
    
    // Eka Yulianti (EY014) - Aman
    _, err = tx.Exec(`INSERT INTO examination_records (patient_id, age, examination_date, tb, bb, lila, imt, is_ttd_rutin, hemoglobin_result) VALUES ('EY014', '16.6 Tahun', '2025-09-20', 160, 52, 25.0, 20.3, true, '{"averageHb": 12.5}');`); if err != nil { tx.Rollback(); log.Fatal(err) }

    // --- DATA PMT ---
    pmtItems := []PmtItem{
        {IconName: "food_bank_outlined", Title: "Recovery Supplementary Food", Description: "Given to malnourished children.", StockCount: 15, TargetGroup: "child", SubItemTitle: "Nutritious Biscuits", SubItemDescription: "Nutrient-dense intake for children"},
        {IconName: "medication_liquid_outlined", Title: "Iron Folic Acid Tablets (TTD)", Description: "≥90 tablets during pregnancy. ≥30 per month.", StockCount: 120, TargetGroup: "pregnantWoman", SubItemTitle: "Fe Tablets", SubItemDescription: "Prevention of anemia in pregnant women"},
        {IconName: "bakery_dining_outlined", Title: "Recovery PMT for Pregnant Women", Description: "Given to pregnant women with Chronic Energy Deficiency (KEK).", StockCount: 10, TargetGroup: "pregnantWoman", SubItemTitle: "High-Calorie Biscuits", SubItemDescription: "Additional nutritional intake"},
        {IconName: "medication_outlined", Title: "TTD for Adolescent Girls", Description: "1 tablet every week throughout the year.", StockCount: 250, TargetGroup: "adolescentGirl", SubItemTitle: "Adolescent Fe Tablets", SubItemDescription: "Weekly anemia prevention"},
    }
    for _, item := range pmtItems {
        _, err = tx.Exec(`INSERT INTO pmt_items (icon_name, title, description, stock_count, target_group, sub_item_title, sub_item_description) VALUES ($1, $2, $3, $4, $5, $6, $7)`, item.IconName, item.Title, item.Description, item.StockCount, item.TargetGroup, item.SubItemTitle, item.SubItemDescription)
        if err != nil { tx.Rollback(); log.Fatal("Gagal seed pmt items: ", err) }
    }
    
    // --- DATA KEHADIRAN (DIPERBANYAK) ---
    // Menggunakan tanggal tetap agar konsisten untuk testing
    fixedToday := "2025-10-12" 
    attendanceQuery := `INSERT INTO attendance (patient_id, date, status) VALUES 
        ('AK002', $1, 'Present'), 
        ('SA007', $1, 'Present'), 
        ('RS005', '2025-10-01', 'Present'), 
        ('NS001', $1, 'Waiting'), 
        ('BS003', $1, 'Waiting'),
        ('SA004', '2025-10-10', 'Present'),
        ('CC013', $1, 'Present'),
        ('LI011', $1, 'Waiting'),
        ('PA006', $1, 'Absent'),
        ('MW012', '2025-09-28', 'Present');`
    _, err = tx.Exec(attendanceQuery, fixedToday)
    if err != nil {
        tx.Rollback()
        log.Fatal(err)
    }

    if err := tx.Commit(); err != nil {
        log.Fatal("Gagal commit transaksi seeding: ", err)
    }
    fmt.Println("Seeding data dummy selesai.")
}
func main() {
    a := App{}; a.Initialize(); createTables(a.DB); seedData(a.DB); a.Run(":8080")
}