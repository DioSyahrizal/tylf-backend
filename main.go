package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
	postgresStorage "github.com/gofiber/storage/postgres/v3"
	"github.com/gofiber/template/html/v2"
	"github.com/spf13/viper"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ==== MODELS ==== //

type User struct {
	ID        int            `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" form:"name" validate:"gte=6,lte=32" gorm:"not null"`
	Email     string         `json:"email" form:"email" validate:"required,email" gorm:"not null"`
	Password  string         `json:"-" form:"password" validate:"required,gte=8" gorm:"not null,column:password"`
	Phone     int            `json:"phone" form:"phone" validate:"required,number,min=12" gorm:"not null"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"nullable;index"`
}

type ResponseGoogleInfo struct {
	FamilyName string `json:"family_name"`
	GivenName  string `json:"given_name"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	Picture    string `json:"picture"`
	Email      string `json:"email"`
}

type ResponseGoogle struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
}

type Env struct {
	DB_HOST              string `mapstructure:"DB_HOST"`
	DB_USER              string `mapstructure:"DB_USER"`
	DB_PASSWORD          string `mapstructure:"DB_PASSWORD"`
	DB_NAME              string `mapstructure:"DB_NAME"`
	DB_PORT              int    `mapstructure:"DB_PORT"`
	APP_ENV              string `mapstructure:"APP_ENV"`
	GOOGLE_CLIENT_ID     string `mapstructure:"GOOGLE_CLIENT_ID"`
	GOOGLE_CLIENT_SECRET string `mapstructure:"GOOGLE_CLIENT_SECRET"`
}

// ==== MAIN ==== //

func main() {

	env := Env{}
	viper.SetConfigFile(".env")

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal("Can't find the file .env : ", err)
	}

	err = viper.Unmarshal(&env)
	if err != nil {
		log.Fatal("Environment can't be loaded: ", err)
	}

	if env.APP_ENV == "development" {
		log.Println("The App is running in development env")
	}

	// Database connection
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Jakarta",
		env.DB_HOST, env.DB_USER, env.DB_PASSWORD, env.DB_NAME, env.DB_PORT)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect to database")
	}

	// Auto migrate User model
	db.AutoMigrate(&User{})

	// Initialize default config

	// Initialize custom config
	storage := postgresStorage.New(postgresStorage.Config{
		Host:       env.DB_HOST,
		Port:       env.DB_PORT,
		Username:   env.DB_USER,
		Password:   env.DB_PASSWORD,
		Database:   env.DB_NAME,
		SSLMode:    "disable",
		Table:      "fiber_storage",
		Reset:      false,
		GCInterval: 10 * time.Second,
	})

	store := session.New(session.Config{
		Storage: storage,
	})

	// Set up Fiber with HTML view engine
	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	// ==== ROUTES ==== //

	app.Get("/login", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)

		userID := sess.Get("user_id")

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get user ID from store",
			})
		}
		if userID != nil {
			return c.Redirect("/protected")
		}
		// Render the login page

		return c.Render("login", fiber.Map{
			"Title":          "Hello, World!",
			"GoogleClientId": env.GOOGLE_CLIENT_ID,
		})
	})

	app.Get("/", func(c *fiber.Ctx) error {
		var users []User
		if err := db.Find(&users).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to fetch user",
			})
		}
		return c.Status(200).JSON(users)
	})

	// ==== GOOGLE OAUTH CALLBACK ==== //

	app.Get("/auth/google/callback", func(c *fiber.Ctx) error {
		code := c.Query("code")
		if code == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "Missing code",
			})
		}

		// Step 1: Exchange code for access token
		tokenURL := "https://oauth2.googleapis.com/token"
		redirectURL := "http://localhost:8080/auth/google/callback"

		formData := url.Values{}
		formData.Set("grant_type", "authorization_code")
		formData.Set("code", code)
		formData.Set("client_id", env.GOOGLE_CLIENT_ID)
		formData.Set("client_secret", env.GOOGLE_CLIENT_SECRET)
		formData.Set("redirect_uri", redirectURL)

		reqBody := strings.NewReader(formData.Encode())
		req, _ := http.NewRequest(http.MethodPost, tokenURL, reqBody)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		client := &http.Client{}
		tokenResp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer tokenResp.Body.Close()

		if tokenResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(tokenResp.Body)
			fmt.Println("Token error:", string(body))
			return fmt.Errorf("failed to get access token")
		}

		var tokenData ResponseGoogle
		if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
			return err
		}

		// Step 2: Fetch user info from Google
		userInfoURL := fmt.Sprintf("https://www.googleapis.com/oauth2/v1/userinfo?access_token=%s", tokenData.AccessToken)
		userReq, _ := http.NewRequest(http.MethodGet, userInfoURL, nil)
		userReq.Header.Add("Content-Type", "application/json")
		userReq.Header.Add("Accept", "application/json")

		userClient := &http.Client{}
		userResp, err := userClient.Do(userReq)
		if err != nil {
			return err
		}
		defer userResp.Body.Close()

		if userResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(userResp.Body)
			fmt.Println("User info error:", string(body))
			return fmt.Errorf("failed to fetch user info")
		}

		var userInfo ResponseGoogleInfo
		if err := json.NewDecoder(userResp.Body).Decode(&userInfo); err != nil {
			return err
		}

		var user User

		checkExist := db.Where("email = ?", userInfo.Email).First(&user)
		if checkExist.Error != nil {
			// User not found, create a new user
			newUser := User{
				Name:      userInfo.Name,
				Email:     userInfo.Email,
				Phone:     0, // Set default phone number or handle it as needed
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Password:  "",
			}

			if err := db.Create(&newUser).Error; err != nil {
				return err
			}
			fmt.Println("User created:", newUser)
		} else {
			// User exists, update user info if needed
			existingUser := User{}
			if err := db.Where("email = ?", userInfo.Email).First(&existingUser).Error; err != nil {
				return err
			}
		}

		sess, err := store.Get(c)

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get session",
			})
		}

		sess.Set("user_id", user.ID)
		if err := sess.Save(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to save session",
			})
		}

		return c.Status(200).JSON(fiber.Map{
			"code":         code,
			"access_token": tokenData.AccessToken,
			"data":         user,
		})
	})

	// protected route
	app.Get("/protected", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		userID := sess.Get("user_id")

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get session",
			})
		}

		if userID == nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}
		return c.Status(200).JSON(fiber.Map{
			"message": "Welcome to the protected route!",
			"user_id": userID,
		})
	})

	// Logout route
	app.Get("/logout", func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		userID := sess.Get("user_id")

		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to get user ID from store",
			})
		}
		if userID == nil {
			return c.Status(401).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		sess.Delete("user_id")

		// Save the session after deleting the key
		if err := sess.Save(); err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to save session",
			})
		}

		return c.Status(200).JSON(fiber.Map{
			"message": "Logged out successfully",
		})
	})

	// Start server
	app.Listen(":8080")
}
