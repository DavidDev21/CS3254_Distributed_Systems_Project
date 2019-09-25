/*
	Name: David Zheng
	Course: CS3254

	main.go will serve as the starting point for the web app
*/

package main

import ("fmt"
		// "bufio"
		// "net"
		// "os"
		"github.com/kataras/iris"
		"flag"
		CO "CS3254_Project/custom_objects"
		)


// IRIS import is being really weird. Visual Code highlights red indicating a problem, but there is none

// Global Var
var database []CO.Product

// Removes an item from the database based on the product ID
// the ... acts like a spread operator in javascript
// append takes a variable amount of arguments and ... allows use to the db slice input
// if no ID found, nothing happens
func removeItem(db []CO.Product, id int) []CO.Product {
	for i, record := range(db) {
		if record.ID == id {
			db = append(db[:i], db[i+1:]...)
		} 
	}

	return db
}

// Does binary search
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
	// Defines flag
	var portNum int
	flag.IntVar(&portNum, "listen", 8080, "this is the port number the application will listen on")
	// var portNum = flag.Int("listen", 8080, "this is the port number the application will listen on") // returns an int pointer to the flag

	// Parases the command line for flags
	flag.Parse()
	fmt.Println(portNum)

	database = append(database, CO.NewProduct(999, "MacBook Air 2019", "This is the new Apple MacBook Air 2019, with Great Specs!", "New","Apple"))
	database = append(database, CO.NewProduct(300, "Nintendo Switch", "This is the most awesome switch in the world", "Good", "Nintendo (the guy next door)"))
	// database[0].PrintProduct()
	// fmt.Println(database)
	// database = removeItem(database, 1)
	// fmt.Println(database)


	// IRIS setup
	app := iris.Default()
	serverAddr := fmt.Sprintf(":%d", portNum)

	// View template registeration
	app.RegisterView(iris.HTML("./views", ".html"))
	// Tells IRIS where are the styles folder
	app.HandleDir("/styles", "./views/styles")
	app.HandleDir("edit/styles", "./views/styles") // Not sure why it would try to get the styles from a different route for edit pages

	// GET method for route "/"
	app.Handle("GET", "/", func(ctx iris.Context) {
		ctx.ViewData("database", database)
		ctx.View("index.html")
	})

	// GET product description page
	app.Handle("GET", "/detail/{ID: int}", func(ctx iris.Context) {
	})

	// GET edit product page
	app.Handle("GET", "/edit/{ID: int}", func(ctx iris.Context) {
		id, err := ctx.Params().GetInt("ID")
		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			item := getItem(&database, id)
			fmt.Println(item.Condition)
			ctx.ViewData("product", *item)
			ctx.View("editProduct.html")
		}
	})
	// POST edit product page
	app.Handle("POST", "/edit/{ID: int}", func(ctx iris.Context) {
		id, err := ctx.Params().GetInt("ID")

		if (err != nil) {
			ctx.Redirect("/", 400)
		} else {
			item := getItem(&database, id)
		
			name := ctx.PostValue("name")
			seller := ctx.PostValue("seller")
			condition := ctx.PostValue("condition")
			description := ctx.PostValue("description")
			price, _ := ctx.PostValueInt("price")

			item.Name = name
			item.Seller = seller
			item.Condition = condition
			item.Description = description
			item.Price = price
		
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
		//fmt.Println(name, seller, condition, description, price)
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
