-- name: GetUserByEmail :one
SELECT id FROM users WHERE email = sqlc.arg(email);

-- name: CreateUser :one
INSERT INTO users (name, email)
VALUES (
    sqlc.arg(name),
    sqlc.arg(email)
)
RETURNING id;