package custom_objects

import ("fmt")

var ProductID = 0

// Struct for product
type Product struct {
	ID int
	Price int
	Name string
	Description string
	Condition string
	Seller string
}

// Constructor for the Product Struct
func NewProduct (price int, name, description, condition, seller string) Product {
	// Closure to increment product ID (could overflow but dont think we would hit that limit)
	getID := func () int {
		ProductID++
		return ProductID
	}
	if price <= 0 {
		price = 0
	}
	newProduct := Product{}
	newProduct.ID = getID()
	newProduct.Price = price
	newProduct.Name = name
	newProduct.Description = description
	newProduct.Condition = condition
	newProduct.Seller = seller
	return newProduct
}

func (self *Product) PrintProduct () {
	fmt.Printf("Product ID: %d \n",self.ID)
	fmt.Printf("Price: %d \n", self.Price)
	fmt.Printf("Product Name: %s \n", self.Name)
	fmt.Printf("Description: %s \n", self.Description)
	fmt.Printf("Condition: %s \n", self.Condition)
	fmt.Printf("Seller: %s \n", self.Seller)
}