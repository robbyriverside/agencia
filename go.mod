module github.com/robbyriverside/agencia

go 1.24.0

require (
	github.com/jessevdk/go-flags v1.6.1
	github.com/joho/godotenv v1.5.1
	github.com/sashabaranov/go-openai v1.38.2
	gopkg.in/yaml.v3 v3.0.1
)

require golang.org/x/sys v0.21.0 // indirect

replace github.com/robbyriverside/agencia => ./
