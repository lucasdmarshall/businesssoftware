package tasks

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"name/backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB           *pgxpool.Pool
	Auth         auth.Handler
	StoragePath  string
	ClamScanPath string
}

type Task struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DueAt            *time.Time `json:"due_at"`
	AssignedTo       *string    `json:"assigned_to"`
	AssigneeName     string     `json:"assignee_name,omitempty"`
	RecurrenceRule   *string    `json:"recurrence_rule"`
	RecurrenceNextAt *time.Time `json:"recurrence_next_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type createRequest struct {
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DueAt            *time.Time `json:"due_at"`
	AssignedTo       *string    `json:"assigned_to"`
	RecurrenceNextAt *time.Time `json:"recurrence_next_at"`
	RecurrenceRule   *string    `json:"recurrence_rule"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "organization"
	}
	if scope != "own" && scope != "department" && scope != "organization" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be own, department, or organization"})
		return
	}
	filter := `t.organization_id = $1`
	args := []any{user.OrganizationID}
	if scope == "own" {
		filter += ` AND (t.created_by = $2 OR t.assigned_to = $2)`
		args = append(args, user.ID)
	}
	if scope == "department" {
		filter += ` AND EXISTS (SELECT 1 FROM user_departments current_ud JOIN user_departments target_ud ON target_ud.department_id=current_ud.department_id WHERE current_ud.user_id=$2 AND target_ud.user_id=COALESCE(t.assigned_to,t.created_by))`
		args = append(args, user.ID)
	}
	rows, err := h.DB.Query(r.Context(), `SELECT t.id, t.title, t.description, t.status, t.priority, t.due_at, t.assigned_to, u.display_name, t.recurrence_rule, t.recurrence_next_at, t.created_at, t.updated_at FROM tasks t LEFT JOIN users u ON u.id=t.assigned_to WHERE `+filter+` ORDER BY t.updated_at DESC LIMIT 500`, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load tasks"})
		return
	}
	defer rows.Close()
	tasks := make([]Task, 0)
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.Priority, &task.DueAt, &task.AssignedTo, &task.AssigneeName, &task.RecurrenceRule, &task.RecurrenceNextAt, &task.CreatedAt, &task.UpdatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read tasks"})
			return
		}
		tasks = append(tasks, task)
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (h Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input createRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Title) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if input.Status == "" {
		input.Status = "todo"
	}
	if input.Priority == "" {
		input.Priority = "normal"
	}
	var task Task
	err = h.DB.QueryRow(r.Context(), `INSERT INTO tasks (organization_id, created_by, title, description, status, priority, due_at, assigned_to, recurrence_rule, recurrence_next_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at,created_at,updated_at`, user.OrganizationID, user.ID, strings.TrimSpace(input.Title), input.Description, input.Status, input.Priority, input.DueAt, input.AssignedTo, input.RecurrenceRule, input.RecurrenceNextAt).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.Priority, &task.DueAt, &task.AssignedTo, &task.RecurrenceRule, &task.RecurrenceNextAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not create task"})
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
		return
	}
	var input createRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Title) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
		return
	}
	if input.Status == "" {
		input.Status = "todo"
	}
	if input.Priority == "" {
		input.Priority = "normal"
	}
	var task Task
	err = h.DB.QueryRow(r.Context(), `UPDATE tasks SET title=$1,description=$2,status=$3,priority=$4,due_at=$5,assigned_to=$6,recurrence_rule=$7,recurrence_next_at=$8,updated_at=NOW() WHERE id=$9 AND organization_id=$10 RETURNING id,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at,created_at,updated_at`, strings.TrimSpace(input.Title), input.Description, input.Status, input.Priority, input.DueAt, input.AssignedTo, input.RecurrenceRule, input.RecurrenceNextAt, id, user.OrganizationID).Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.Priority, &task.DueAt, &task.AssignedTo, &task.RecurrenceRule, &task.RecurrenceNextAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h Handler) GenerateRecurring(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	type recurring struct {
		ID, CreatedBy, Title, Description, Status, Priority string
		DueAt, AssignedTo, NextAt                           *time.Time
		Rule                                                string
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id,created_by,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at FROM tasks WHERE organization_id=$1 AND recurrence_rule IS NOT NULL AND recurrence_next_at IS NOT NULL AND recurrence_next_at <= NOW() ORDER BY recurrence_next_at LIMIT 100`, user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load recurring tasks"})
		return
	}
	defer rows.Close()
	items := make([]recurring, 0)
	for rows.Next() {
		var item recurring
		if err := rows.Scan(&item.ID, &item.CreatedBy, &item.Title, &item.Description, &item.Status, &item.Priority, &item.DueAt, &item.AssignedTo, &item.Rule, &item.NextAt); err == nil {
			items = append(items, item)
		}
	}
	created := 0
	for _, item := range items {
		next := *item.NextAt
		switch item.Rule {
		case "daily":
			next = next.AddDate(0, 0, 1)
		case "weekly":
			next = next.AddDate(0, 0, 7)
		case "monthly":
			next = next.AddDate(0, 1, 0)
		default:
			continue
		}
		_, err = h.DB.Exec(r.Context(), `INSERT INTO tasks (organization_id,created_by,title,description,status,priority,due_at,assigned_to,recurrence_rule,recurrence_next_at) VALUES ($1,$2,$3,$4,'todo',$5,$6,$7,$8,$9)`, user.OrganizationID, item.CreatedBy, item.Title, item.Description, item.Priority, next, item.AssignedTo, item.Rule, next)
		if err == nil {
			_, _ = h.DB.Exec(r.Context(), `UPDATE tasks SET recurrence_rule=NULL,recurrence_next_at=NULL,updated_at=NOW() WHERE id=$1`, item.ID)
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"generated": created})
}

type Comment struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}
type commentRequest struct {
	Body string `json:"body"`
}

func (h Handler) Comments(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	taskID := r.PathValue("id")
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT c.id,c.author_id,u.display_name,c.body,c.created_at FROM task_comments c JOIN users u ON u.id=c.author_id WHERE c.task_id=$1 AND c.organization_id=$2 ORDER BY c.created_at`, taskID, user.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load comments"})
			return
		}
		defer rows.Close()
		items := make([]Comment, 0)
		for rows.Next() {
			var item Comment
			if err := rows.Scan(&item.ID, &item.AuthorID, &item.AuthorName, &item.Body, &item.CreatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read comments"})
				return
			}
			items = append(items, item)
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	var input commentRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Body) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "comment body is required"})
		return
	}
	var item Comment
	err = h.DB.QueryRow(r.Context(), `INSERT INTO task_comments (task_id,organization_id,author_id,body) SELECT $1,$2,$3,$4 WHERE EXISTS (SELECT 1 FROM tasks WHERE id=$1 AND organization_id=$2) RETURNING id,author_id,author_id::text,body,created_at`, taskID, user.OrganizationID, user.ID, strings.TrimSpace(input.Body)).Scan(&item.ID, &item.AuthorID, &item.AuthorName, &item.Body, &item.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

type Attachment struct {
	ID          string    `json:"id"`
	FileName    string    `json:"file_name"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h Handler) Attachments(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	taskID := r.PathValue("id")
	if r.Method == http.MethodGet {
		rows, err := h.DB.Query(r.Context(), `SELECT id,file_name,content_type,size_bytes,created_at FROM task_attachments WHERE task_id=$1 AND organization_id=$2 ORDER BY created_at`, taskID, user.OrganizationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load attachments"})
			return
		}
		defer rows.Close()
		items := make([]Attachment, 0)
		for rows.Next() {
			var item Attachment
			if err := rows.Scan(&item.ID, &item.FileName, &item.ContentType, &item.SizeBytes, &item.CreatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read attachments"})
				return
			}
			items = append(items, item)
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file is too large; maximum is 10 MB"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()
	if header.Filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file name is required"})
		return
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create storage key"})
		return
	}
	storageKey := fmt.Sprintf("%x", token[:])
	if err = os.MkdirAll(h.StoragePath, 0700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not prepare file storage"})
		return
	}
	path := filepath.Join(h.StoragePath, storageKey)
	dest, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save file"})
		return
	}
	size, copyErr := io.Copy(dest, io.LimitReader(file, 10<<20+1))
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil || size > 10<<20 {
		_ = os.Remove(path)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file is too large or could not be saved"})
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	scanStatus := "unscanned"
	if h.ClamScanPath != "" {
		if scanErr := exec.CommandContext(r.Context(), h.ClamScanPath, "--no-summary", path).Run(); scanErr == nil {
			scanStatus = "clean"
		} else if exitErr, ok := scanErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			scanStatus = "infected"
		}
	}
	var item Attachment
	err = h.DB.QueryRow(r.Context(), `INSERT INTO task_attachments (task_id,organization_id,uploaded_by,file_name,storage_key,content_type,size_bytes,scan_status) SELECT $1,$2,$3,$4,$5,$6,$7,$8 WHERE EXISTS (SELECT 1 FROM tasks WHERE id=$1 AND organization_id=$2) RETURNING id,file_name,content_type,size_bytes,created_at`, taskID, user.OrganizationID, user.ID, filepath.Base(header.Filename), storageKey, contentType, size, scanStatus).Scan(&item.ID, &item.FileName, &item.ContentType, &item.SizeBytes, &item.CreatedAt)
	if err != nil {
		_ = os.Remove(path)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h Handler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	taskID, attachmentID := r.PathValue("id"), r.PathValue("attachmentID")
	var storageKey, fileName, contentType, scanStatus string
	err = h.DB.QueryRow(r.Context(), `SELECT a.storage_key,a.file_name,a.content_type,a.scan_status FROM task_attachments a WHERE a.id=$1 AND a.task_id=$2 AND a.organization_id=$3`, attachmentID, taskID, user.OrganizationID).Scan(&storageKey, &fileName, &contentType, &scanStatus)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attachment not found"})
		return
	}
	if scanStatus != "clean" {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "attachment is not available until security scanning is complete"})
		return
	}
	path := filepath.Join(h.StoragePath, filepath.Base(storageKey))
	if filepath.Base(storageKey) != storageKey {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attachment not found"})
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attachment file is unavailable"})
		return
	}
	_, _ = h.DB.Exec(r.Context(), `INSERT INTO audit_logs (organization_id,actor_id,action,entity_type,entity_id) VALUES ($1,$2,'attachment.download','task_attachment',$3)`, user.OrganizationID, user.ID, attachmentID)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filepath.Base(fileName), `"`, `_`)+`"`)
	http.ServeFile(w, r, path)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
