package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"

	"github.com/oarkflow/json/examples/models"
	"github.com/oarkflow/json/jsonschema/v2"
)

func schemaValidator(file string) fiber.Handler {
	sampleSchema, _ := os.ReadFile(file)
	compiler := v2.NewCompiler()
	schema, err := compiler.Compile(sampleSchema)
	return func(ctx *fiber.Ctx) error {
		if err != nil {
			return err
		}
		var p models.Person
		err = schema.UnmarshalFiberCtx(ctx, &p)
		if err != nil {
			return err
		}
		ctx.Locals("person", p)
		return ctx.Next()
	}
}

func main() {
	app := fiber.New()
	app.Post("/", schemaValidator("schema_request.json"), func(c *fiber.Ctx) error {
		person := c.Locals("person").(models.Person)
		return c.JSON(person)
	})
	log.Fatal(app.Listen(":3000"))
}
