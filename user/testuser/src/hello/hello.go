package hello

func Hello(place string) (greeting string, e error) {
	greeting = "Hello " + place
	return greeting, nil
}
