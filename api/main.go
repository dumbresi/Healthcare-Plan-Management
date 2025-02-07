package main

import (
	"github.com/dumbresi/Healthcare-Plan-Management/api/config"
	"github.com/gofiber/fiber/v2"
)

func main() {
	config.InitRedis()
	app := fiber.New()
	routes.SetupRoutes(app)
	app.Listen(":8080")
}
