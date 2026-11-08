package http

import "context"

type readiness interface{ Ping(context.Context) error }
