package handlers

import "github.com/gofiber/fiber/v2"

// Home renders the empty shorten form.
func (h *Handler) Home(c *fiber.Ctx) error {
	return c.Render("index", fiber.Map{})
}

// ShortenForm handles the browser form post and renders the result on the same
// page. Validation problems come back as a message above the form with the
// input preserved, so the user never loses what they typed.
func (h *Handler) ShortenForm(c *fiber.Ctx) error {
	var req urlRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).Render("index", fiber.Map{
			"Error": "could not read the submitted form",
		})
	}

	record, err := h.createURL(req.URL)
	if err != nil {
		status := fiber.StatusInternalServerError
		message := "something went wrong, please try again"
		if fe, ok := err.(*fiber.Error); ok {
			status = fe.Code
			message = fe.Message
		}
		return c.Status(status).Render("index", fiber.Map{
			"Error": message,
			"Input": req.URL,
		})
	}

	return c.Render("index", fiber.Map{
		"ShortURL": h.shortURL(record.ShortenCode),
		"Original": record.URL,
	})
}
