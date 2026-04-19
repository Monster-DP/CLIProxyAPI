package api

import (
	"time"

	"github.com/gin-gonic/gin"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

func requestTimingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c != nil && c.Request != nil {
			ctx := c.Request.Context()
			if _, ok := cliproxyexecutor.RequestStart(ctx); !ok {
				c.Request = c.Request.WithContext(cliproxyexecutor.WithRequestStart(ctx, time.Now()))
			}
		}
		c.Next()
	}
}
