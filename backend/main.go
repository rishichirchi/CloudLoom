package main

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rishichirchi/cloudloom/config"
	"github.com/rishichirchi/cloudloom/route"
)

func main() {
	env_error := godotenv.Load()
	if env_error != nil {
		panic("Error loading .env file")
	}
	// Initialize AWS configuration
	config.InitAWS()

	// Set up Gin router
	// gin.SetMode(gin.ReleaseMode) // Set Gin to release mode for production
	app := gin.Default()

	// Configure CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:3001", "https://your-frontend-domain.com"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	route.SetupRoutes(app)

	app.Run(":5000")
}
