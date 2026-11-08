package get

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Kirill1705/m3305_vikhrenko_go/internal/http/response"
)

type Handler struct{ usecase usecase }

func New(usecase usecase) *Handler { return &Handler{usecase: usecase} }
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	result, err := h.usecase.Execute(r.Context(), id)
	if err != nil {
		response.Error(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, response.JSONType, response.Trip(result))
}
