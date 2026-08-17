package rooms

import "time"

type Room struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	Slug             string     `json:"slug"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	ManagedDefault   bool       `json:"managed_default"`
	CreatedByActorID *string    `json:"created_by_actor_id,omitempty"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastMessageAt    *time.Time `json:"last_message_at,omitempty"`
}

type Participant struct {
	ActorID     string `json:"actor_id"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	AgentType   string `json:"agent_type,omitempty"`
	Handle      string `json:"handle"`
	Online      bool   `json:"online"`
}

type MessageMention struct {
	MentionID   string `json:"mention_id"`
	ActorID     string `json:"actor_id"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
}

type Message struct {
	ID                  string           `json:"id"`
	WorkspaceID         string           `json:"workspace_id"`
	RoomID              string           `json:"room_id"`
	AuthorActorID       string           `json:"author_actor_id"`
	AuthorDisplayName   string           `json:"author_display_name"`
	AuthorKind          string           `json:"author_kind"`
	AuthorSessionID     *string          `json:"author_session_id,omitempty"`
	ReplyToMessageID    *string          `json:"reply_to_message_id,omitempty"`
	ThreadRootMessageID *string          `json:"thread_root_message_id,omitempty"`
	Body                string           `json:"body"`
	Mentions            []MessageMention `json:"mentions"`
	CreatedAt           time.Time        `json:"created_at"`
}

type Mention struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	RoomID      string     `json:"room_id"`
	RoomName    string     `json:"room_name"`
	Message     Message    `json:"message"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	DismissedAt *time.Time `json:"dismissed_at,omitempty"`
}

type CreateRoomInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
}

type CreateRoomResult struct {
	Room     Room
	Replayed bool
}

type CreateMessageInput struct {
	Body             string   `json:"body"`
	ReplyToMessageID string   `json:"reply_to_message_id,omitempty"`
	MentionActorIDs  []string `json:"mention_actor_ids,omitempty"`
	AuthorSessionID  string   `json:"author_session_id,omitempty"`
}

type CreateMessageResult struct {
	Message  Message
	Replayed bool
}

type MessageListOptions struct {
	BeforeMessageID     string
	ThreadRootMessageID string
	Query               string
	Limit               int
}

type InboxOptions struct {
	WorkspaceID string
	Status      string
	Limit       int
}

type MentionStatusInput struct {
	Status string `json:"status"`
}
