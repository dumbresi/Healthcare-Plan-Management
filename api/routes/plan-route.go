package routes

import (
	"github.com/dumbresi/Healthcare-Plan-Management/api/controllers"
	"github.com/dumbresi/Healthcare-Plan-Management/api/middleware"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App) {
	api := app.Group("/api/v1")
	api.Post("/plans",middleware.AuthMiddleware, controllers.CreatePlan)
	api.Get("/plans",middleware.AuthMiddleware,controllers.GetAllPlans)
	api.Get("/plans/:id",middleware.AuthMiddleware, controllers.GetPlan)
	api.Delete("/plans/:id",middleware.AuthMiddleware, controllers.DeletePlan)
	api.Patch("/plans/:id",middleware.AuthMiddleware, controllers.PatchPlan)
}
