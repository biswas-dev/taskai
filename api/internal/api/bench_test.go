package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// setupBenchServer creates a test server and seeds it with a user and project.
// Returns the server, userID, and projectID. Caller must call ts.Close().
func setupBenchServer(b *testing.B) (*TestServer, int64, int64) {
	b.Helper()
	ts := NewTestServer(b)
	userID := ts.CreateTestUser(b, "bench@test.com", "password123")
	projectID := ts.CreateTestProject(b, userID, "Bench Project")
	return ts, userID, projectID
}

func BenchmarkHandleListTasks(b *testing.B) {
	for _, n := range []int{10, 50, 100, 500} {
		b.Run(fmt.Sprintf("tasks=%d", n), func(b *testing.B) {
			ts, userID, projectID := setupBenchServer(b)
			defer ts.Close()

			// Seed tasks
			for i := 0; i < n; i++ {
				ts.CreateTestTask(b, projectID, fmt.Sprintf("Task %d", i+1))
			}

			rec, req := ts.MakeAuthRequest(b, http.MethodGet, "/api/projects/1/tasks", nil, userID,
				map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rec.Body.Reset()
				ts.HandleListTasks(rec, req)
			}
		})
	}
}

func BenchmarkHandleListProjects(b *testing.B) {
	ts, userID, _ := setupBenchServer(b)
	defer ts.Close()

	// Create a few more projects
	for i := 2; i <= 10; i++ {
		ts.CreateTestProject(b, userID, fmt.Sprintf("Project %d", i))
	}

	rec, req := ts.MakeAuthRequest(b, http.MethodGet, "/api/projects", nil, userID, nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		ts.HandleListProjects(rec, req)
	}
}

func BenchmarkHandleCreateTask(b *testing.B) {
	ts, userID, projectID := setupBenchServer(b)
	defer ts.Close()

	body := map[string]interface{}{
		"title":    "Benchmark task",
		"priority": "medium",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec, req := ts.MakeAuthRequest(b, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/tasks", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateTask(rec, req)
	}
}

func BenchmarkHandleGetTaskByNumber(b *testing.B) {
	ts, userID, projectID := setupBenchServer(b)
	defer ts.Close()

	ts.CreateTestTask(b, projectID, "Target Task")

	rec, req := ts.MakeAuthRequest(b, http.MethodGet,
		fmt.Sprintf("/api/projects/%d/tasks/1", projectID), nil, userID,
		map[string]string{
			"projectId":  fmt.Sprintf("%d", projectID),
			"taskNumber": "1",
		})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		ts.HandleGetTaskByNumber(rec, req)
	}
}

func BenchmarkHandleSearchWiki(b *testing.B) {
	ts, userID, _ := setupBenchServer(b)
	defer ts.Close()

	body := SearchWikiRequest{
		Query: "test",
		Limit: 20,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec, req := ts.MakeAuthRequest(b, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)
	}
}

func BenchmarkHandleGlobalSearch(b *testing.B) {
	ts, userID, projectID := setupBenchServer(b)
	defer ts.Close()

	// Seed some tasks for search
	for i := 0; i < 20; i++ {
		ts.CreateTestTask(b, projectID, fmt.Sprintf("Searchable task %d", i+1))
	}

	body := map[string]interface{}{
		"query": "Searchable",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec, req := ts.MakeAuthRequest(b, http.MethodPost, "/api/search", body, userID, nil)
		ts.HandleGlobalSearch(rec, req)
	}
}

// BenchmarkJSONMarshal measures JSON encoding overhead for task list responses
func BenchmarkJSONMarshal(b *testing.B) {
	tasks := make([]map[string]interface{}, 100)
	for i := range tasks {
		tasks[i] = map[string]interface{}{
			"id":          i + 1,
			"task_number": i + 1,
			"title":       fmt.Sprintf("Task %d", i+1),
			"status":      "todo",
			"priority":    "medium",
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		json.Marshal(tasks)
	}
}
