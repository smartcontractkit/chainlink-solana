package fakes

import (
	"github.com/gin-gonic/gin"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/fake"
)

const FakeServicePort = 9111

func RegisterRoutes() error {
	return fake.Func("POST", "/mockserver-bridge", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"data": map[string]any{"result": 5}})
	})
}
