package repository

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

type VecRepo struct {
	db *sql.DB
}

func NewVecRepo(db *sql.DB) *VecRepo {
	return &VecRepo{db: db}
}

var vecDimRe = regexp.MustCompile(`(?i)FLOAT\[(\d+)\]`)

// Init creates the vec0 table, or reports the dimension it was created with so a
// mismatch with the configured embedding model is not silently accepted. An index
// that is still empty is recreated instead: there is nothing in it to lose.
func (r *VecRepo) Init(embedDim int) error {
	if err := r.create(embedDim); err != nil {
		return err
	}

	current, ok, err := r.dimension()
	if err != nil {
		return err
	}
	if !ok || current == embedDim {
		return nil
	}
	// Switching embedding model is a normal setup step, and an index nobody has
	// filled yet holds nothing worth protecting.
	empty, err := r.isEmpty()
	if err != nil {
		return err
	}
	if !empty {
		return fmt.Errorf("vec_memory dimension mismatch: table is FLOAT[%d] but config requests %d, rebuild the index after changing embed_dim", current, embedDim)
	}
	return r.Reset(embedDim)
}

func (r *VecRepo) create(embedDim int) error {
	if _, err := r.db.Exec(fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_memory USING vec0(
			id          TEXT PRIMARY KEY,
			embedding   FLOAT[%d],
			person_id   TEXT,
			chunk_type  TEXT,
			source_id   TEXT
		)`, embedDim)); err != nil {
		return fmt.Errorf("create vec_memory table: %w", err)
	}
	return nil
}

// Reset drops the index and recreates it at embedDim; callers rebuild every row
// afterwards, which is how a new embedding model or dimension takes effect.
func (r *VecRepo) Reset(embedDim int) error {
	if _, err := r.db.Exec(`DROP TABLE IF EXISTS vec_memory`); err != nil {
		return fmt.Errorf("drop vec_memory: %w", err)
	}
	return r.create(embedDim)
}

func (r *VecRepo) isEmpty() (bool, error) {
	var rows int
	if err := r.db.QueryRow(`SELECT count(*) FROM vec_memory`).Scan(&rows); err != nil {
		return false, fmt.Errorf("count vec_memory: %w", err)
	}
	return rows == 0, nil
}

func (r *VecRepo) dimension() (int, bool, error) {
	var schema string
	err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'vec_memory'`).Scan(&schema)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read vec_memory schema: %w", err)
	}
	match := vecDimRe.FindStringSubmatch(schema)
	if match == nil {
		return 0, false, nil
	}
	dim, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false, fmt.Errorf("parse vec_memory dimension: %w", err)
	}
	return dim, true, nil
}

func (r *VecRepo) Insert(id string, embedding []float32, personID, chunkType, sourceID string) error {
	// vec0 enforces the text primary key but ignores INSERT OR REPLACE conflict
	// resolution, so an upsert has to delete the previous row first.
	if _, err := r.db.Exec(`DELETE FROM vec_memory WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete vec_memory %s: %w", id, err)
	}
	const query = `
		INSERT INTO vec_memory (id, embedding, person_id, chunk_type, source_id)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := r.db.Exec(query, id, SerializeEmbedding(embedding), personID, chunkType, sourceID); err != nil {
		return fmt.Errorf("insert vec_memory: %w", err)
	}
	return nil
}

// SearchByPerson runs a person-scoped KNN query restricted to one chunk type.
// The chunk filter lives inside the virtual table so the limit applies to rows of
// that type only — otherwise trait rows could fill the whole result set.
func (r *VecRepo) SearchByPerson(queryEmbedding []float32, personID, chunkType string, limit int) ([]VecResult, error) {
	rows, err := r.db.Query(`
		SELECT source_id, chunk_type, distance
		FROM vec_memory
		WHERE embedding MATCH ? AND person_id = ? AND chunk_type = ?
		ORDER BY distance
		LIMIT ?`, SerializeEmbedding(queryEmbedding), personID, chunkType, limit)
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
	return results, rows.Err()
}

func (r *VecRepo) DeleteBySourceID(sourceID string) error {
	if _, err := r.db.Exec(`DELETE FROM vec_memory WHERE source_id = ?`, sourceID); err != nil {
		return fmt.Errorf("delete vec_memory: %w", err)
	}
	return nil
}

func (r *VecRepo) DeleteByPerson(personID string) error {
	if _, err := r.db.Exec(`DELETE FROM vec_memory WHERE person_id = ?`, personID); err != nil {
		return fmt.Errorf("delete vec_memory by person: %w", err)
	}
	return nil
}

// SerializeEmbedding packs float32s into the little-endian blob layout vec0 expects;
// database/sql has no conversion for []float32 and rejects it as an argument.
func SerializeEmbedding(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

type VecResult struct {
	SourceID  string
	ChunkType string
	Distance  float64
}
