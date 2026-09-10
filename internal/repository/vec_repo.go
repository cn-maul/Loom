package repository

import (
	"database/sql"
	"fmt"
)

type VecRepo struct {
	db *sql.DB
}

func NewVecRepo(db *sql.DB) *VecRepo {
	return &VecRepo{db: db}
}

func (r *VecRepo) Init(embedDim int) error {
	query := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_memory USING vec0(
			id          TEXT PRIMARY KEY,
			embedding   FLOAT[%d],
			person_id   TEXT,
			chunk_type  TEXT,
			source_id   TEXT
		)
	`, embedDim)

	_, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("create vec_memory table: %w", err)
	}
	return nil
}

func (r *VecRepo) Insert(id string, embedding []float32, personID, chunkType, sourceID string) error {
	query := `
		INSERT OR REPLACE INTO vec_memory (id, embedding, person_id, chunk_type, source_id)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := r.db.Exec(query, id, embedding, personID, chunkType, sourceID)
	if err != nil {
		return fmt.Errorf("insert vec_memory: %w", err)
	}
	return nil
}

func (r *VecRepo) SearchByPerson(queryEmbedding []float32, personID string, limit int) ([]VecResult, error) {
	query := `
		SELECT source_id, chunk_type, distance
		FROM vec_memory
		WHERE embedding MATCH ? AND person_id = ?
		ORDER BY distance
		LIMIT ?
	`
	rows, err := r.db.Query(query, queryEmbedding, personID, limit)
	if err != nil {
		return nil, fmt.Errorf("search vec_memory: %w", err)
	}
	defer rows.Close()

	var results []VecResult
	for rows.Next() {
		var result VecResult
		if err := rows.Scan(&result.SourceID, &result.ChunkType, &result.Distance); err != nil {
			return nil, fmt.Errorf("scan vec_result: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (r *VecRepo) DeleteBySourceID(sourceID string) error {
	query := `DELETE FROM vec_memory WHERE source_id = ?`
	_, err := r.db.Exec(query, sourceID)
	if err != nil {
		return fmt.Errorf("delete vec_memory: %w", err)
	}
	return nil
}

type VecResult struct {
	SourceID  string
	ChunkType string
	Distance  float64
}