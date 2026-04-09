package models

// Recipe mirrors captaincore_recipes from the PHP schema.
type Recipe struct {
	RecipeID  uint   `gorm:"primaryKey;column:recipe_id" json:"recipe_id,string"`
	Title     string `gorm:"column:title" json:"title"`
	Content   string `gorm:"column:content;type:text" json:"content"`
	CreatedAt string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt string `gorm:"column:updated_at" json:"updated_at"`
}

func (Recipe) TableName() string {
	return "captaincore_recipes"
}

// GetRecipeByID returns a recipe by its recipe_id.
func GetRecipeByID(recipeID uint) (*Recipe, error) {
	var recipe Recipe
	result := DB.Where("recipe_id = ?", recipeID).First(&recipe)
	if result.Error != nil {
		return nil, result.Error
	}
	return &recipe, nil
}

// UpsertRecipe inserts or updates a recipe by recipe_id.
func UpsertRecipe(recipe *Recipe) error {
	var existing Recipe
	result := DB.Where("recipe_id = ?", recipe.RecipeID).First(&existing)
	if result.Error != nil {
		// Insert new
		return DB.Create(recipe).Error
	}
	// Update existing
	return DB.Model(&existing).Updates(recipe).Error
}
