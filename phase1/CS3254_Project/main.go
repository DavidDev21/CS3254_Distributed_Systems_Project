/*
	Name: David Zheng (dz1063)
	Course: CS3254
	Project Part 1
	main.go will serve as the starting point for the web app
*/

package main

import ("fmt"
		"github.com/kataras/iris"
		"flag"
		CO "CS3254_Project/custom_objects"
		)

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
func getItem(db *[]CO.Product, target int) *CO.Product {
	left, right := 0, len(*db)-1

	for left <= right {
		mid := (left+right)/2
		// return the address to the product object in the database
		if ((*db)[mid].ID == target) {
			return &(*db)[mid]
		} else if ((*db)[mid].ID < target) {
			left = mid+1
		} else {
			right = mid-1
		}
	}
	return nil
}

func main() {
	// Defines the listen flag
	var portNum int
	flag.IntVar(&portNum, "listen", 8080, "this is the port number the application will listen on")

	// Parases the command line for flags
	flag.Parse()
	fmt.Println(portNum)

	// Add some initial items into our db
	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))

	// IRIS setup
	app := iris.Default()
	serverAddr := fmt.Sprintf(":%d", portNum)

	// View template registeration
	app.RegisterView(iris.HTML("./views", ".html"))

	// Tells IRIS where is the styles folder
	app.HandleDir("/styles", "./views/styles")

	// GET method for route "/"
	app.Handle("GET", "/", func(ctx iris.Context) {
		ctx.ViewData("database", database)
		ctx.View("index.html")
	})

	// GET edit product page
	app.Handle("GET", "/edit/{ID: int}", func(ctx iris.Context) {
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			item := getItem(&database, id)
			ctx.ViewData("product", *item)
			ctx.View("editProduct.html")
		}
	})

	// POST edit product page
	// Handles the update of a product
	app.Handle("POST", "/edit/{ID: int}", func(ctx iris.Context) {
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			item := getItem(&database, id)

			item.Name = ctx.PostValue("name")
			item.Seller = ctx.PostValue("seller")
			item.Condition = ctx.PostValue("condition")
			item.Description = ctx.PostValue("description")
			item.Price, _ = ctx.PostValueInt("price")
		
			ctx.Redirect("/", 302)
		}
	})

	// GET Add product
	app.Handle("GET", "/createProduct", func(ctx iris.Context) {
		ctx.View("createProduct.html")
	})

	// POST Add product
	app.Handle("POST", "/createProduct", func(ctx iris.Context) {
		name := ctx.PostValue("name")
		seller := ctx.PostValue("seller")
		condition := ctx.PostValue("condition")
		description := ctx.PostValue("description")
		price, _ := ctx.PostValueInt("price")

		database = append(database, CO.NewProduct(price, name, description, condition, seller))
		// Go back to home
		ctx.Redirect("/", 302)
	})

	// Delete product
	app.Handle("POST", "/delete/{ID: int}", func(ctx iris.Context) {
		id, err := ctx.Params().GetInt("ID")
		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			database = removeItem(database, id)
			ctx.Redirect("/", 302)
		}
	})
	// IRIS listening on port
	app.Run(iris.Addr(serverAddr), iris.WithoutServerError(iris.ErrServerClosed))
}
