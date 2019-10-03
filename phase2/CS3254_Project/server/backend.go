/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 2 (Backend)
	backend.go will serve as the starting point for the backend server
*/
package main

import ("fmt"
		"flag"
		"net"
		"net/rpc"
		"os"
		"errors"
		CO "CS3254_Project/custom_objects")

// Global Var
var database []CO.Product

// Removes an item from the database based on the product ID
// the ... acts like a spread operator in javascript
// append takes a variable amount of arguments and ... allows use to the db slice as input
// if no ID found, nothing happens
func removeItem(db []CO.Product, id int) []CO.Product {
	for i, record := range(db) {
		if record.ID == id {
			db = append(db[:i], db[i+1:]...)
		} 
	}

	return db
}

// Does binary search to find the item in our DB list
// Items on the list are always in ascending order
func getItem(db *[]CO.Product, target int) (*CO.Product, error) {
	left, right := 0, len(*db)-1

	for left <= right {
		mid := (left+right)/2
		// return the address to the product object in the database
		if ((*db)[mid].ID == target) {
			return &(*db)[mid], nil
		} else if ((*db)[mid].ID < target) {
			left = mid+1
		} else {
			right = mid-1
		}
	}
	return &CO.Product{}, errors.New("ID not found")
}

/* RPC HANDLERS */
type DB struct {}
type Request struct {
	ProductID int
	ProductInfo CO.Product
}
type Response struct {
	ProductList []CO.Product
	ProductItem CO.Product
}

// Returns the whole database to the client
func (t *DB) GetProductList(req Request, res *Response) error {
	res.ProductList = database
	fmt.Println("returning a database to frontend")
	return nil
}

// Returns a Product to the client based on given Product ID
func (t *DB) GetProduct(req Request, res *Response) error {
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	res.ProductItem = *item
	fmt.Println("return a product to frontend")
	return nil
}

// Edit Product Info based on given Product ID
func (t *DB) EditProduct(req Request, res *Response) error {
	item, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	item.Name = req.ProductInfo.Name
	item.Seller = req.ProductInfo.Seller
	item.Condition = req.ProductInfo.Condition
	item.Description = req.ProductInfo.Description
	item.Price = req.ProductInfo.Price

	fmt.Println("Editing Product ID: ", req.ProductID)
	return nil
}

// Creates a new product and append to database based on info given
func (t *DB) AddProduct(req Request, res *Response) error {
	price := req.ProductInfo.Price
	name := req.ProductInfo.Name
	description := req.ProductInfo.Description
	condition := req.ProductInfo.Condition
	seller := req.ProductInfo.Seller

	database = append(database, CO.NewProduct(price, name, description, condition, seller))
	return nil
}

// Delete a product from database based on ProductID
func (t *DB) DeleteProduct(req Request, res *Response) error {
	// Check if ID is valid
	_, err := getItem(&database, req.ProductID)

	if (err != nil) {
		return err
	}

	database = removeItem(database, req.ProductID)
	return nil
}

func main() {
	// Add some initial items into our db
	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))

	// Register a handler
	rpc.Register(new(DB))

	// Defines the listen flag
	var portNum int
	flag.IntVar(&portNum, "listen", 8090, "this is the port number the application will listen on")

	// Parases the command line for flags
	flag.Parse()
	fmt.Println(portNum)

	servicePort := fmt.Sprintf(":%d", portNum)

	// Listen on the servicePort
	listener, err := net.Listen("tcp", servicePort)

	if (err != nil){
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Server now listening on", servicePort)
	rpc.Accept(listener)
}