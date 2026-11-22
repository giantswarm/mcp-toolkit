package httpx

import (
	"encoding/json"
	"maps"
	"net/http"
)

// Problem is an RFC 9457 problem detail. Extensions cannot override its
// standard members. Detail must be safe to disclose to the caller.
type Problem struct {
	Type       string
	Title      string
	Status     int
	Detail     string
	Instance   string
	Extensions map[string]any
}

// MarshalJSON encodes problem details with their extension members.
func (p Problem) MarshalJSON() ([]byte, error) {
	fields := maps.Clone(p.Extensions)
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["type"], fields["title"], fields["status"] = p.Type, p.Title, p.Status
	delete(fields, "detail")
	delete(fields, "instance")
	if p.Detail != "" {
		fields["detail"] = p.Detail
	}
	if p.Instance != "" {
		fields["instance"] = p.Instance
	}
	return json.Marshal(fields)
}

// WriteProblem writes application/problem+json with an error status. Invalid
// statuses or unencodable extensions produce a generic internal error.
func WriteProblem(writer http.ResponseWriter, problem Problem) {
	if problem.Status < 400 || problem.Status > 599 {
		problem = Problem{Status: http.StatusInternalServerError}
	}
	if problem.Type == "" {
		problem.Type = "about:blank"
	}
	if problem.Title == "" {
		problem.Title = http.StatusText(problem.Status)
	}
	encoded, err := json.Marshal(problem)
	if err != nil {
		problem = Problem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError}
		encoded, _ = json.Marshal(problem)
	}
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(problem.Status)
	_, _ = writer.Write(append(encoded, '\n'))
}
