package user_handler

import (
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
)

// [Athene] Handlers de catálogo de produtos (WhatsApp Business).

func catalogInstance(ctx *gin.Context) (*instance_model.Instance, bool) {
	getInstance := ctx.MustGet("instance")
	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	return instance, true
}

// Create a catalog product
// @Summary Create a catalog product
// @Description Cria um produto no catálogo do WhatsApp Business da instância.
// @Description Forneça a imagem via `imageBase64` (recomendado) ou `imageUrl`.
// @Description `price` é em miliunidades da moeda (R$ 10,00 = 10000).
// @Tags Catalog
// @Accept json
// @Produce json
// @Param message body user_service.ProductCreateStruct true "Product data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /catalog/product [post]
func (u *userHandler) CreateProduct(ctx *gin.Context) {
	instance, ok := catalogInstance(ctx)
	if !ok {
		return
	}
	var data *user_service.ProductCreateStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	product, err := u.userService.CreateProduct(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": product})
}

// Update a catalog product
// @Summary Update a catalog product
// @Description Edita um produto existente no catálogo. Atualização parcial:
// @Description só os campos preenchidos mudam. Envie imagem só para trocá-la.
// @Tags Catalog
// @Accept json
// @Produce json
// @Param message body user_service.ProductUpdateStruct true "Product data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /catalog/product [put]
func (u *userHandler) UpdateProduct(ctx *gin.Context) {
	instance, ok := catalogInstance(ctx)
	if !ok {
		return
	}
	var data *user_service.ProductUpdateStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	product, err := u.userService.UpdateProduct(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": product})
}

// Get the catalog
// @Summary Get the catalog
// @Description Lista os produtos do catálogo do WhatsApp Business da instância.
// @Tags Catalog
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /catalog/products [get]
func (u *userHandler) GetCatalog(ctx *gin.Context) {
	instance, ok := catalogInstance(ctx)
	if !ok {
		return
	}
	products, err := u.userService.GetCatalog(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": products})
}

// Delete catalog products
// @Summary Delete catalog products
// @Description Remove um ou mais produtos do catálogo por ID.
// @Tags Catalog
// @Accept json
// @Produce json
// @Param message body user_service.ProductDeleteStruct true "Product IDs"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /catalog/product [delete]
func (u *userHandler) DeleteProducts(ctx *gin.Context) {
	instance, ok := catalogInstance(ctx)
	if !ok {
		return
	}
	var data *user_service.ProductDeleteStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deleted, err := u.userService.DeleteProducts(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "deleted": deleted})
}
