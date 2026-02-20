package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/weiawesome/wes-io-live/search-service/internal/domain"
)

type esSearchRepository struct {
	client     *elasticsearch.Client
	indexUsers string
	indexRooms string
}

// NewESSearchRepository creates a new Elasticsearch-based search repository.
func NewESSearchRepository(client *elasticsearch.Client, indexUsers, indexRooms string) SearchRepository {
	return &esSearchRepository{
		client:     client,
		indexUsers: indexUsers,
		indexRooms: indexRooms,
	}
}

func (r *esSearchRepository) SearchUsers(ctx context.Context, query string, offset, limit int) ([]domain.UserResult, int, error) {
	body := map[string]interface{}{
		"from": offset,
		"size": limit,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"username", "display_name", "email"},
			},
		},
		"_source": map[string]interface{}{
			"excludes": []string{"password_hash"},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(r.indexUsers),
		r.client.Search.WithBody(bytes.NewReader(data)),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search users: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, 0, fmt.Errorf("elasticsearch error: %s", res.String())
	}

	var result esResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	users := make([]domain.UserResult, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		var user domain.UserResult
		if err := json.Unmarshal(hit.Source, &user); err != nil {
			continue
		}
		users = append(users, user)
	}

	return users, result.Hits.Total.Value, nil
}

func (r *esSearchRepository) SearchRooms(ctx context.Context, query string, offset, limit int) ([]domain.RoomResult, int, error) {
	body := map[string]interface{}{
		"from": offset,
		"size": limit,
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"title", "description", "owner_username", "tags"},
			},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(r.indexRooms),
		r.client.Search.WithBody(bytes.NewReader(data)),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search rooms: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, 0, fmt.Errorf("elasticsearch error: %s", res.String())
	}

	var result esResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	rooms := make([]domain.RoomResult, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		var room domain.RoomResult
		if err := json.Unmarshal(hit.Source, &room); err != nil {
			continue
		}
		rooms = append(rooms, room)
	}

	return rooms, result.Hits.Total.Value, nil
}

// esResponse is the generic Elasticsearch search response structure.
type esResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source json.RawMessage `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}
