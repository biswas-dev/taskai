package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func setupBenchDB(b *testing.B) *DB {
	b.Helper()
	logger := zaptest.NewLogger(b)

	cfg := Config{
		DBPath:         ":memory:",
		MigrationsPath: "./migrations",
	}

	database, err := New(cfg, logger)
	if err != nil {
		b.Fatalf("Failed to create test database: %v", err)
	}

	return database
}

func BenchmarkDBWrite(b *testing.B) {
	database := setupBenchDB(b)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a user and project for FK constraints
	var userID int64
	err := database.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`,
		"bench@test.com", "$2a$04$fake",
	).Scan(&userID)
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	var projectID int64
	err = database.QueryRowContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?) RETURNING id`,
		userID, "Bench Project", "desc",
	).Scan(&projectID)
	if err != nil {
		b.Fatalf("Failed to create project: %v", err)
	}

	stmt, err := database.PrepareContext(ctx,
		`INSERT INTO tasks (project_id, task_number, title, status, priority) VALUES (?, ?, ?, 'todo', 'medium')`)
	if err != nil {
		b.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := stmt.ExecContext(ctx, projectID, i+1, fmt.Sprintf("Task %d", i+1))
		if err != nil {
			b.Fatalf("INSERT failed at i=%d: %v", i, err)
		}
	}
}

func BenchmarkDBRead(b *testing.B) {
	database := setupBenchDB(b)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed a user
	var userID int64
	err := database.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`,
		"bench@test.com", "$2a$04$fake",
	).Scan(&userID)
	if err != nil {
		b.Fatalf("Failed to create user: %v", err)
	}

	// Seed a project
	var projectID int64
	err = database.QueryRowContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?) RETURNING id`,
		userID, "Bench Project", "desc",
	).Scan(&projectID)
	if err != nil {
		b.Fatalf("Failed to create project: %v", err)
	}

	// Seed a task
	var taskID int64
	err = database.QueryRowContext(ctx,
		`INSERT INTO tasks (project_id, task_number, title, status, priority) VALUES (?, 1, 'Read me', 'todo', 'medium') RETURNING id`,
		projectID,
	).Scan(&taskID)
	if err != nil {
		b.Fatalf("Failed to create task: %v", err)
	}

	stmt, err := database.PrepareContext(ctx,
		`SELECT id, title, status, priority FROM tasks WHERE id = ?`)
	if err != nil {
		b.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var id int64
		var title, status, priority string
		err := stmt.QueryRowContext(ctx, taskID).Scan(&id, &title, &status, &priority)
		if err != nil {
			b.Fatalf("SELECT failed: %v", err)
		}
	}
}

func BenchmarkDBBulkRead(b *testing.B) {
	for _, n := range []int{10, 50, 100, 500} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			database := setupBenchDB(b)
			defer database.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Seed user + project
			var userID int64
			database.QueryRowContext(ctx,
				`INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`,
				"bench@test.com", "$2a$04$fake",
			).Scan(&userID)

			var projectID int64
			database.QueryRowContext(ctx,
				`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?) RETURNING id`,
				userID, "Bench Project", "desc",
			).Scan(&projectID)

			// Seed n tasks
			for i := 0; i < n; i++ {
				_, err := database.ExecContext(ctx,
					`INSERT INTO tasks (project_id, task_number, title, status, priority) VALUES (?, ?, ?, 'todo', 'medium')`,
					projectID, i+1, fmt.Sprintf("Task %d", i+1),
				)
				if err != nil {
					b.Fatalf("Failed to seed task %d: %v", i, err)
				}
			}

			stmt, err := database.PrepareContext(ctx,
				`SELECT id, task_number, title, status, priority FROM tasks WHERE project_id = ?`)
			if err != nil {
				b.Fatalf("Failed to prepare statement: %v", err)
			}
			defer stmt.Close()

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rows, err := stmt.QueryContext(ctx, projectID)
				if err != nil {
					b.Fatalf("SELECT failed: %v", err)
				}
				count := 0
				for rows.Next() {
					var id, taskNum int64
					var title, status, priority string
					rows.Scan(&id, &taskNum, &title, &status, &priority)
					count++
				}
				rows.Close()
				if count != n {
					b.Fatalf("Expected %d rows, got %d", n, count)
				}
			}
		})
	}
}
