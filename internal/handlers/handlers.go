package handlers

import (
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/AP1493/go-urlshortner/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const shortCodeLength = 8

type Handler struct {
	db      *gorm.DB
	rdb     *redis.Client
	baseURL string
}

func NewHandler(db *gorm.DB, rdb *redis.Client) *Handler {
	return &Handler{db: db, rdb: rdb, baseURL: baseURL()}
}

// baseURL is the public origin short links are built from. It has to be
// configured because the app may sit behind a proxy or a different port than it
// listens on.
func baseURL() string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("BASE_URL")), "/"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

type urlRequest struct {
	URL string `json:"url" form:"url"`
}

// normalizeURL trims the input and validates that it is an absolute http(s) URL.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("url is not valid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("url must start with http:// or https://")
	}
	if parsed.Host == "" {
		return "", errors.New("url must contain a host")
	}

	return raw, nil
}

// GenerateRandomString returns a random base62 string of n characters. It uses
// crypto/rand so short codes cannot be guessed or enumerated.
func GenerateRandomString(n int) (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b), nil
}

// createURL validates the raw input, mints a short code and stores the row. It
// is shared by the JSON API and the HTML form, and always fails with a
// *fiber.Error so callers can tell a bad input (400) from a server fault (500).
func (h *Handler) createURL(raw string) (models.URL, error) {
	value, err := normalizeURL(raw)
	if err != nil {
		return models.URL{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	shortenCode, err := GenerateRandomString(shortCodeLength)
	if err != nil {
		return models.URL{}, fiber.NewError(fiber.StatusInternalServerError, "could not generate short code")
	}

	record := models.URL{URL: value, ShortenCode: shortenCode}
	if err := h.db.Create(&record).Error; err != nil {
		return models.URL{}, fiber.NewError(fiber.StatusInternalServerError, "could not save URL")
	}

	return record, nil
}

// shortURL builds the full link handed back to the user for a short code.
func (h *Handler) shortURL(code string) string {
	return h.baseURL + "/shorten/" + code
}

// Create stores a new URL.
func (h *Handler) Create(c *fiber.Ctx) error {
	var req urlRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	record, err := h.createURL(req.URL)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(record)
}

// Get redirects a short code to its URL. The code -> URL lookup is served from
// Redis when cached, falling back to Postgres; the visit is always counted in
// Postgres.
func (h *Handler) Get(c *fiber.Ctx) error {
	code := c.Params("shorten_code")

	target, cached := h.cachedURL(c.Context(), code)
	if !cached {
		var record models.URL
		if err := h.db.First(&record, "shorten_code = ?", code).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fiber.NewError(fiber.StatusNotFound, "URL not found")
			}
			return fiber.NewError(fiber.StatusInternalServerError, "could not fetch URL")
		}
		target = record.URL
		h.cacheURL(c.Context(), code, target)
	}

	result := h.db.Model(&models.URL{}).Where("shorten_code = ?", code).UpdateColumn("count", gorm.Expr("count + 1"))
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not update URL count")
	}
	if result.RowsAffected == 0 {
		// Only reachable on a cache hit for a row that is gone, e.g. a delete
		// whose cache eviction failed. Drop the stale entry.
		h.uncacheURL(c.Context(), code)
		return fiber.NewError(fiber.StatusNotFound, "URL not found")
	}

	return c.Redirect(target, fiber.StatusFound)
}

// Delete removes a URL by its ID.
func (h *Handler) Delete(c *fiber.Ctx) error {
	result := h.db.Delete(&models.URL{}, "shorten_code = ?", c.Params("shorten_code"))
	if result.Error != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not delete URL")
	}
	if result.RowsAffected == 0 {
		return fiber.NewError(fiber.StatusNotFound, "URL not found")
	}
	h.uncacheURL(c.Context(), c.Params("shorten_code"))

	return c.SendStatus(fiber.StatusNoContent)
}

// Stats returns the stored record, including its access count, without
// redirecting or counting the lookup as a visit.
func (h *Handler) Stats(c *fiber.Ctx) error {
	var record models.URL
	if err := h.db.First(&record, "shorten_code = ?", c.Params("shorten_code")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "URL not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "could not fetch URL")
	}

	return c.JSON(record)
}

// Update repoints an existing short code at a new URL. The short code itself is
// left alone so links already handed out keep working.
func (h *Handler) Update(c *fiber.Ctx) error {
	var req urlRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	value, err := normalizeURL(req.URL)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// Load the row first so the response carries the real id, count and
	// timestamps, and so a missing short code is a 404 rather than a silent
	// no-op update.
	var record models.URL
	if err := h.db.First(&record, "shorten_code = ?", c.Params("shorten_code")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "URL not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "could not fetch URL")
	}

	if err := h.db.Model(&record).Update("url", value).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "could not update URL")
	}
	h.uncacheURL(c.Context(), record.ShortenCode)

	return c.JSON(record)
}
