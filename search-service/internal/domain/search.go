package domain

// SearchRequest is the unified search request.
type SearchRequest struct {
	Query  string `form:"q" binding:"required"`
	Type   string `form:"type"`   // "user" | "room" | "" (all)
	Offset int    `form:"offset"`
	Limit  int    `form:"limit"`
}

// SearchResponse is the unified search response.
type SearchResponse struct {
	Users []UserResult `json:"users,omitempty"`
	Rooms []RoomResult `json:"rooms,omitempty"`
	Total int          `json:"total"`
}

// UserResult maps to ES index cdc-public-users (GORM column names from CDC).
type UserResult struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// RoomResult maps to ES index cdc-public-rooms (GORM column names from CDC).
type RoomResult struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	OwnerID       string   `json:"owner_id"`
	OwnerUsername string   `json:"owner_username"`
	Status        string   `json:"status"`
	ViewerCount   int      `json:"viewer_count"`
	Tags          []string `json:"tags"`
}
