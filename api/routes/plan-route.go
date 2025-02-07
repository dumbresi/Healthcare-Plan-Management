package routes

import (
	// "healthcare-api/controllers"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")
	// api.Post("/plans", controllers.CreatePlan)
	// api.Get("/plans/:id", controllers.GetPlan)
	// api.Delete("/plans/:id", controllers.DeletePlan)
}
