CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    apple_id VARCHAR(255) NOT NULL,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    device_id VARCHAR(255) NOT NULL DEFAULT '',
    dnd_start TIME,
    dnd_end TIME,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    deletion_scheduled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_active_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_users_apple_id ON users(apple_id) WHERE is_deleted = FALSE;
CREATE INDEX idx_users_last_active ON users(last_active_at);
CREATE INDEX idx_users_deletion_scheduled ON users(deletion_scheduled_at) WHERE is_deleted = TRUE;
