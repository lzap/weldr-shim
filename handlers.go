package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"github.com/osbuild/blueprint/pkg/blueprint"
)

// responseError represents a Weldr API error
type responseError struct {
	ID  string `json:"id"`
	Msg string `json:"msg"`
}

// statusResponse represents a simple success/fail response
type statusResponse struct {
	Status bool            `json:"status"`
	Errors []responseError `json:"errors,omitempty"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// writeError writes an error response
func writeError(w http.ResponseWriter, status int, id, msg string) {
	writeJSON(w, status, statusResponse{
		Status: false,
		Errors: []responseError{{ID: id, Msg: msg}},
	})
}

// writeSuccess writes a success response
func writeSuccess(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, statusResponse{Status: true})
}

// handleNotImplemented returns 501 for unimplemented endpoints
func handleNotImplemented(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NotImplemented",
		fmt.Sprintf("Endpoint %s %s is not implemented", r.Method, r.URL.Path))
}

// statusV0Response represents the /api/status response
type statusV0Response struct {
	API           string   `json:"api"`
	DBSupported   bool     `json:"db_supported"`
	DBVersion     string   `json:"db_version"`
	SchemaVersion string   `json:"schema_version"`
	Backend       string   `json:"backend"`
	Build         string   `json:"build"`
	Messages      []string `json:"msgs"`
}

// handleStatus returns the service status
func handleStatus(w http.ResponseWriter, r *http.Request) {
	response := statusV0Response{
		API:           "1",
		DBSupported:   true,
		DBVersion:     "0",
		SchemaVersion: "0",
		Backend:       "weldr-shim",
		Build:         "1.0.0",
		Messages:      []string{},
	}

	writeJSON(w, http.StatusOK, response)
}

// blueprintsListResponse represents the response for /api/v1/blueprints/list
type blueprintsListResponse struct {
	Total      uint     `json:"total"`
	Offset     uint     `json:"offset"`
	Limit      uint     `json:"limit"`
	Blueprints []string `json:"blueprints"`
}

// handleBlueprintsList returns all blueprint names
func handleBlueprintsList(w http.ResponseWriter, r *http.Request) {
	names, err := ListBlueprints(*cacheDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "BlueprintsError", err.Error())
		return
	}

	response := blueprintsListResponse{
		Total:      uint(len(names)),
		Offset:     0,
		Limit:      uint(len(names)),
		Blueprints: names,
	}

	writeJSON(w, http.StatusOK, response)
}

// blueprintsInfoResponse represents the response for /api/v1/blueprints/info
type blueprintsInfoResponse struct {
	Blueprints []blueprint.Blueprint `json:"blueprints"`
	Changes    []blueprintChange     `json:"changes"`
	Errors     []responseError       `json:"errors"`
}

type blueprintChange struct {
	Changed bool   `json:"changed"`
	Name    string `json:"name"`
}

// handleBlueprintsInfo returns blueprint details
func handleBlueprintsInfo(w http.ResponseWriter, r *http.Request) {
	// Extract names from path: /api/v1/blueprints/info/name1,name2
	path := r.URL.Path
	prefix := "/api/v1/blueprints/info/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	namesParam := strings.TrimPrefix(path, prefix)
	if namesParam == "" {
		writeError(w, http.StatusBadRequest, "UnknownBlueprint", "No blueprints specified")
		return
	}

	names := strings.Split(namesParam, ",")

	var blueprints []blueprint.Blueprint
	var changes []blueprintChange
	var errors []responseError

	for _, name := range names {
		// Validate blueprint name
		if err := ValidateBlueprintName(name); err != nil {
			errors = append(errors, responseError{
				ID:  "InvalidName",
				Msg: fmt.Sprintf("%s: invalid blueprint name", name),
			})
			continue
		}

		data, err := LoadBlueprint(*cacheDir, name)
		if err != nil {
			errors = append(errors, responseError{
				ID:  "UnknownBlueprint",
				Msg: fmt.Sprintf("%s: blueprint not found", name),
			})
			continue
		}

		var bp blueprint.Blueprint
		if err := json.Unmarshal(data, &bp); err != nil {
			errors = append(errors, responseError{
				ID:  "BlueprintsError",
				Msg: fmt.Sprintf("%s: failed to parse blueprint", name),
			})
			continue
		}

		blueprints = append(blueprints, bp)
		changes = append(changes, blueprintChange{Changed: false, Name: bp.Name})
	}

	response := blueprintsInfoResponse{
		Blueprints: blueprints,
		Changes:    changes,
		Errors:     errors,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleBlueprintsNew creates or updates a blueprint
func handleBlueprintsNew(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")

	if contentType != "application/json" && contentType != "text/x-toml" {
		writeError(w, http.StatusBadRequest, "BlueprintsError", "Content-Type must be application/json or text/x-toml")
		return
	}

	if r.ContentLength == 0 {
		writeError(w, http.StatusBadRequest, "BlueprintsError", "Missing blueprint")
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BlueprintsError", "Failed to read request body")
		return
	}

	// Parse blueprint (support both JSON and TOML)
	var bp blueprint.Blueprint
	if contentType == "application/json" {
		if err := json.Unmarshal(body, &bp); err != nil {
			writeError(w, http.StatusBadRequest, "BlueprintsError", fmt.Sprintf("Invalid JSON: %v", err))
			return
		}
	} else {
		// For TOML, use toml.Unmarshal
		if err := toml.Unmarshal(body, &bp); err != nil {
			writeError(w, http.StatusBadRequest, "BlueprintsError", fmt.Sprintf("Invalid TOML: %v", err))
			return
		}
	}

	// Validate blueprint name
	if err := ValidateBlueprintName(bp.Name); err != nil {
		writeError(w, http.StatusBadRequest, "BlueprintsError", fmt.Sprintf("Invalid blueprint name: %v", err))
		return
	}

	// Save as JSON
	jsonData, err := json.Marshal(bp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "BlueprintsError", "Failed to marshal blueprint")
		return
	}

	if err := SaveBlueprint(*cacheDir, bp.Name, jsonData); err != nil {
		writeError(w, http.StatusInternalServerError, "BlueprintsError", err.Error())
		return
	}

	writeSuccess(w)
}

// handleBlueprintDelete deletes a blueprint
func handleBlueprintDelete(w http.ResponseWriter, r *http.Request) {
	// Extract name from path: /api/v1/blueprints/delete/name
	path := r.URL.Path
	prefix := "/api/v1/blueprints/delete/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	name := strings.TrimPrefix(path, prefix)
	if name == "" {
		writeError(w, http.StatusBadRequest, "UnknownBlueprint", "No blueprint specified")
		return
	}

	// Validate blueprint name
	if err := ValidateBlueprintName(name); err != nil {
		writeError(w, http.StatusBadRequest, "UnknownBlueprint", fmt.Sprintf("Invalid blueprint name: %v", err))
		return
	}

	if err := DeleteBlueprint(*cacheDir, name); err != nil {
		writeError(w, http.StatusInternalServerError, "BlueprintsError", err.Error())
		return
	}

	writeSuccess(w)
}

// distrosListResponse represents the response for /api/v1/distros/list
type distrosListResponse struct {
	Distros []string `json:"distros"`
}

// handleDistrosList returns available distros
func handleDistrosList(w http.ResponseWriter, r *http.Request) {
	// Get unique distro names
	distroMap := make(map[string]bool)

	for _, entry := range imageTypesCache {
		distroMap[entry.Distro] = true
	}

	// Convert to sorted slice
	distros := make([]string, 0, len(distroMap))
	for distroName := range distroMap {
		distros = append(distros, distroName)
	}
	sort.Strings(distros)

	writeJSON(w, http.StatusOK, distrosListResponse{Distros: distros})
}

// composeTypesResponse represents the response for /api/v1/compose/types
type composeTypesResponse struct {
	Types []composeTypeInfo `json:"types"`
}

type composeTypeInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// handleComposeTypes returns available compose types
func handleComposeTypes(w http.ResponseWriter, r *http.Request) {
	// Get unique image types from image types cache
	imageTypes := make(map[string]bool)
	for _, entry := range imageTypesCache {
		imageTypes[entry.ImageType] = true
	}

	var types []composeTypeInfo
	for typeName := range imageTypes {
		types = append(types, composeTypeInfo{
			Name:    typeName,
			Enabled: true,
		})
	}

	writeJSON(w, http.StatusOK, composeTypesResponse{Types: types})
}

// composeStartRequest represents the request body for starting a compose
type composeStartRequest struct {
	BlueprintName string `json:"blueprint_name"`
	ComposeType   string `json:"compose_type"`
	Branch        string `json:"branch"`
}

// composeStartResponse represents the response for compose start
type composeStartResponse struct {
	BuildID string          `json:"build_id"`
	Status  bool            `json:"status"`
	Errors  []responseError `json:"errors,omitempty"`
}

// handleComposeStart starts a new compose
func handleComposeStart(w http.ResponseWriter, r *http.Request) {
	var req composeStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "Invalid JSON")
		return
	}

	if req.BlueprintName == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "Missing blueprint_name")
		return
	}

	if req.ComposeType == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "Missing compose_type")
		return
	}

	// Load blueprint to get version
	data, err := LoadBlueprint(*cacheDir, req.BlueprintName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "UnknownBlueprint", fmt.Sprintf("Blueprint %s not found", req.BlueprintName))
		return
	}

	var bp blueprint.Blueprint
	if err := json.Unmarshal(data, &bp); err != nil {
		writeError(w, http.StatusInternalServerError, "BlueprintsError", "Failed to parse blueprint")
		return
	}

	// Validate image type exists
	// Note: We're not validating distro/arch here since composer-cli doesn't send them
	// We'll use a default or derive from the system
	found := false
	for _, entry := range imageTypesCache {
		if entry.ImageType == req.ComposeType {
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusBadRequest, "InvalidComposeType", fmt.Sprintf("Unknown compose type: %s", req.ComposeType))
		return
	}

	// Determine distro and arch: prioritize blueprint, fallback to defaults
	distro := bp.Distro
	if distro == "" {
		distro = defaultDistro
	}

	arch := bp.Arch
	if arch == "" {
		arch = defaultArch
	}

	// Create compose
	composeID := uuid.New().String()
	metadata := ComposeMetadata{
		ID:               composeID,
		BlueprintName:    bp.Name,
		BlueprintVersion: bp.Version,
		ComposeType:      req.ComposeType,
		Distro:           distro,
		Arch:             arch,
		Created:          time.Now(),
	}

	if err := CreateCompose(*cacheDir, metadata); err != nil {
		writeError(w, http.StatusInternalServerError, "ComposeError", err.Error())
		return
	}

	response := composeStartResponse{
		BuildID: composeID,
		Status:  true,
	}

	writeJSON(w, http.StatusOK, response)
}

// composeStatusResponse represents the response for /api/v1/compose/status
type composeStatusResponse struct {
	UUIDs []composeStatus `json:"uuids"`
}

type composeStatus struct {
	ID          string  `json:"id"`
	Blueprint   string  `json:"blueprint"`
	Version     string  `json:"version"`
	ComposeType string  `json:"compose_type"`
	ImageSize   uint64  `json:"image_size"`
	QueueStatus string  `json:"queue_status"`
	JobCreated  float64 `json:"job_created"`
	JobStarted  float64 `json:"job_started,omitempty"`
	JobFinished float64 `json:"job_finished,omitempty"`
}

// handleComposeStatus returns status for one or more composes
func handleComposeStatus(w http.ResponseWriter, r *http.Request) {
	// Clean up any stale RUNNING composes from killed processes
	CleanupStaleComposes(*cacheDir)

	// Extract UUIDs from path: /api/v1/compose/status/uuid1,uuid2
	// If no UUIDs specified, return all composes
	path := r.URL.Path
	prefix := "/api/v1/compose/status/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	uuidsParam := strings.TrimPrefix(path, prefix)
	var uuids []string

	if uuidsParam == "" || uuidsParam == "*" {
		// Return all composes
		allUUIDs, err := ListComposes(*cacheDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ComposeError", err.Error())
			return
		}
		uuids = allUUIDs
	} else {
		uuids = strings.Split(uuidsParam, ",")
	}

	var statuses []composeStatus
	for _, id := range uuids {
		metadata, err := LoadComposeMetadata(*cacheDir, id)
		if err != nil {
			// Skip missing composes
			continue
		}

		status, err := LoadComposeStatus(*cacheDir, id)
		if err != nil {
			status = "UNKNOWN"
		}

		cs := composeStatus{
			ID:          metadata.ID,
			Blueprint:   metadata.BlueprintName,
			Version:     metadata.BlueprintVersion,
			ComposeType: metadata.ComposeType,
			QueueStatus: status,
			JobCreated:  float64(metadata.Created.Unix()),
		}

		if !metadata.Started.IsZero() {
			cs.JobStarted = float64(metadata.Started.Unix())
		}
		if !metadata.Finished.IsZero() {
			cs.JobFinished = float64(metadata.Finished.Unix())
		}

		// Get image size if finished
		if status == "FINISHED" {
			_, size, err := GetComposeImagePath(*cacheDir, id)
			if err == nil {
				cs.ImageSize = uint64(size)
			}
		}

		statuses = append(statuses, cs)
	}

	writeJSON(w, http.StatusOK, composeStatusResponse{UUIDs: statuses})
}

// composeInfoResponse represents the response for /api/v1/compose/info
type composeInfoResponse struct {
	ID          string              `json:"id"`
	Config      string              `json:"config"`
	Blueprint   blueprint.Blueprint `json:"blueprint"`
	Commit      string              `json:"commit"`
	Deps        composeDeps         `json:"deps"`
	ComposeType string              `json:"compose_type"`
	QueueStatus string              `json:"queue_status"`
	ImageSize   uint64              `json:"image_size"`
}

type composeDeps struct {
	Packages []any `json:"packages"`
}

// handleComposeInfo returns detailed compose information
func handleComposeInfo(w http.ResponseWriter, r *http.Request) {
	// Clean up any stale RUNNING composes from killed processes
	CleanupStaleComposes(*cacheDir)

	// Extract UUID from path: /api/v1/compose/info/uuid
	path := r.URL.Path
	prefix := "/api/v1/compose/info/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	uuid := strings.TrimPrefix(path, prefix)
	if uuid == "" {
		writeError(w, http.StatusBadRequest, "UnknownUUID", "No UUID specified")
		return
	}

	// Load metadata
	metadata, err := LoadComposeMetadata(*cacheDir, uuid)
	if err != nil {
		writeError(w, http.StatusBadRequest, "UnknownUUID",
			fmt.Sprintf("%s is not a valid build uuid", uuid))
		return
	}

	// Load blueprint
	bpData, err := LoadBlueprint(*cacheDir, metadata.BlueprintName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "InternalError",
			"Failed to load blueprint")
		return
	}

	var bp blueprint.Blueprint
	if err := json.Unmarshal(bpData, &bp); err != nil {
		writeError(w, http.StatusInternalServerError, "InternalError",
			"Failed to parse blueprint")
		return
	}

	// Load status
	status, err := LoadComposeStatus(*cacheDir, uuid)
	if err != nil {
		status = "UNKNOWN"
	}

	response := composeInfoResponse{
		ID:          uuid,
		Config:      "",
		Blueprint:   bp,
		Commit:      "",
		Deps:        composeDeps{Packages: []any{}},
		ComposeType: metadata.ComposeType,
		QueueStatus: status,
		ImageSize:   0,
	}

	writeJSON(w, http.StatusOK, response)
}

// composeQueueResponse represents the response for /api/v1/compose/queue
type composeQueueResponse struct {
	New []composeStatus `json:"new"`
	Run []composeStatus `json:"run"`
}

// handleComposeQueue returns composes in WAITING and RUNNING state
func handleComposeQueue(w http.ResponseWriter, r *http.Request) {
	// Clean up any stale RUNNING composes from killed processes
	CleanupStaleComposes(*cacheDir)

	uuids, err := ListComposes(*cacheDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ComposeError", err.Error())
		return
	}

	var newStatuses []composeStatus
	var runStatuses []composeStatus

	for _, id := range uuids {
		status, err := LoadComposeStatus(*cacheDir, id)
		if err != nil {
			continue
		}

		if status != "WAITING" && status != "RUNNING" {
			continue
		}

		metadata, err := LoadComposeMetadata(*cacheDir, id)
		if err != nil {
			continue
		}

		cs := composeStatus{
			ID:          metadata.ID,
			Blueprint:   metadata.BlueprintName,
			Version:     metadata.BlueprintVersion,
			ComposeType: metadata.ComposeType,
			QueueStatus: status,
			JobCreated:  float64(metadata.Created.Unix()),
		}

		if !metadata.Started.IsZero() {
			cs.JobStarted = float64(metadata.Started.Unix())
		}

		if status == "WAITING" {
			newStatuses = append(newStatuses, cs)
		} else {
			runStatuses = append(runStatuses, cs)
		}
	}

	// Ensure arrays are never null
	if newStatuses == nil {
		newStatuses = []composeStatus{}
	}
	if runStatuses == nil {
		runStatuses = []composeStatus{}
	}

	writeJSON(w, http.StatusOK, composeQueueResponse{New: newStatuses, Run: runStatuses})
}

// composeFinishedResponse represents the response for /api/v1/compose/finished
type composeFinishedResponse struct {
	Finished []composeStatus `json:"finished"`
}

// handleComposeFinished returns composes in FINISHED state
func handleComposeFinished(w http.ResponseWriter, r *http.Request) {
	uuids, err := ListComposes(*cacheDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ComposeError", err.Error())
		return
	}

	var finishedStatuses []composeStatus
	for _, id := range uuids {
		status, err := LoadComposeStatus(*cacheDir, id)
		if err != nil || status != "FINISHED" {
			continue
		}

		metadata, err := LoadComposeMetadata(*cacheDir, id)
		if err != nil {
			continue
		}

		cs := composeStatus{
			ID:          metadata.ID,
			Blueprint:   metadata.BlueprintName,
			Version:     metadata.BlueprintVersion,
			ComposeType: metadata.ComposeType,
			QueueStatus: "FINISHED",
			JobCreated:  float64(metadata.Created.Unix()),
		}

		if !metadata.Started.IsZero() {
			cs.JobStarted = float64(metadata.Started.Unix())
		}
		if !metadata.Finished.IsZero() {
			cs.JobFinished = float64(metadata.Finished.Unix())
		}

		_, size, err := GetComposeImagePath(*cacheDir, id)
		if err == nil {
			cs.ImageSize = uint64(size)
		}

		finishedStatuses = append(finishedStatuses, cs)
	}

	// Ensure array is never null
	if finishedStatuses == nil {
		finishedStatuses = []composeStatus{}
	}

	writeJSON(w, http.StatusOK, composeFinishedResponse{Finished: finishedStatuses})
}

// handleComposeFailed returns composes in FAILED state
func handleComposeFailed(w http.ResponseWriter, r *http.Request) {
	uuids, err := ListComposes(*cacheDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ComposeError", err.Error())
		return
	}

	var failedStatuses []composeStatus
	for _, id := range uuids {
		status, err := LoadComposeStatus(*cacheDir, id)
		if err != nil || status != "FAILED" {
			continue
		}

		metadata, err := LoadComposeMetadata(*cacheDir, id)
		if err != nil {
			continue
		}

		cs := composeStatus{
			ID:          metadata.ID,
			Blueprint:   metadata.BlueprintName,
			Version:     metadata.BlueprintVersion,
			ComposeType: metadata.ComposeType,
			QueueStatus: "FAILED",
			JobCreated:  float64(metadata.Created.Unix()),
		}

		if !metadata.Started.IsZero() {
			cs.JobStarted = float64(metadata.Started.Unix())
		}
		if !metadata.Finished.IsZero() {
			cs.JobFinished = float64(metadata.Finished.Unix())
		}

		failedStatuses = append(failedStatuses, cs)
	}

	writeJSON(w, http.StatusOK, composeStatusResponse{UUIDs: failedStatuses})
}

// handleComposeImage streams the compose image file
func handleComposeImage(w http.ResponseWriter, r *http.Request) {
	// Extract UUID from path: /api/v1/compose/image/uuid
	path := r.URL.Path
	prefix := "/api/v1/compose/image/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	id := strings.TrimPrefix(path, prefix)
	if id == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "No UUID specified")
		return
	}

	// Check compose status
	status, err := LoadComposeStatus(*cacheDir, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "UnknownUUID", fmt.Sprintf("Compose %s not found", id))
		return
	}

	if status != "FINISHED" {
		writeError(w, http.StatusBadRequest, "BuildInWrongState", fmt.Sprintf("Build %s is in %s state", id, status))
		return
	}

	// Get image path
	imagePath, _, err := GetComposeImagePath(*cacheDir, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "UnknownUUID", fmt.Sprintf("Image for %s not found", id))
		return
	}

	// Stream the file
	http.ServeFile(w, r, imagePath)
}

type deletedUUID struct {
	UUID   string `json:"uuid"`
	Status bool   `json:"status"`
}

// handleComposeDelete deletes a compose
func handleComposeDelete(w http.ResponseWriter, r *http.Request) {
	// Extract UUIDs from path: /api/v1/compose/delete/uuid1,uuid2
	path := r.URL.Path
	prefix := "/api/v1/compose/delete/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	uuidsParam := strings.TrimPrefix(path, prefix)
	if uuidsParam == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "No UUIDs specified")
		return
	}

	uuids := strings.Split(uuidsParam, ",")

	var errors []responseError
	deletedCount := 0

	for _, id := range uuids {
		// Check if compose exists
		_, err := LoadComposeStatus(*cacheDir, id)
		if err != nil {
			errors = append(errors, responseError{
				ID:  "UnknownUUID",
				Msg: fmt.Sprintf("%s: compose not found", id),
			})
			continue
		}

		if err := DeleteCompose(*cacheDir, id); err != nil {
			errors = append(errors, responseError{
				ID:  "ComposeError",
				Msg: fmt.Sprintf("%s: %v", id, err),
			})
			continue
		}

		deletedCount++
	}

	response := struct {
		Status bool            `json:"status"`
		Errors []responseError `json:"errors,omitempty"`
		UUIDs  []deletedUUID   `json:"uuids"`
	}{
		Status: len(errors) == 0,
		Errors: errors,
		UUIDs:  make([]deletedUUID, deletedCount),
	}

	writeJSON(w, http.StatusOK, response)
}

// handleComposeCancel cancels a running compose
func handleComposeCancel(w http.ResponseWriter, r *http.Request) {
	// Extract UUIDs from path: /api/v1/compose/cancel/uuid1,uuid2
	path := r.URL.Path
	prefix := "/api/v1/compose/cancel/"
	if !strings.HasPrefix(path, prefix) {
		writeError(w, http.StatusNotFound, "HTTPError", "Not Found")
		return
	}

	uuidsParam := strings.TrimPrefix(path, prefix)
	if uuidsParam == "" {
		writeError(w, http.StatusBadRequest, "InvalidRequest", "No UUIDs specified")
		return
	}

	uuids := strings.Split(uuidsParam, ",")

	var errors []responseError
	canceledCount := 0

	for _, id := range uuids {
		// Check status
		status, err := LoadComposeStatus(*cacheDir, id)
		if err != nil {
			errors = append(errors, responseError{
				ID:  "UnknownUUID",
				Msg: fmt.Sprintf("%s: compose not found", id),
			})
			continue
		}

		if status != "WAITING" && status != "RUNNING" {
			errors = append(errors, responseError{
				ID:  "BuildInWrongState",
				Msg: fmt.Sprintf("%s: compose is %s, cannot cancel", id, status),
			})
			continue
		}

		// Cancel by setting status to FAILED
		if err := UpdateComposeStatus(*cacheDir, id, "FAILED"); err != nil {
			errors = append(errors, responseError{
				ID:  "ComposeError",
				Msg: fmt.Sprintf("%s: %v", id, err),
			})
			continue
		}

		canceledCount++
	}

	response := struct {
		Status bool            `json:"status"`
		Errors []responseError `json:"errors,omitempty"`
		UUIDs  []deletedUUID   `json:"uuids"`
	}{
		Status: len(errors) == 0,
		Errors: errors,
		UUIDs:  make([]deletedUUID, canceledCount),
	}

	writeJSON(w, http.StatusOK, response)
}
