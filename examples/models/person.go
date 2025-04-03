package models

type Name struct {
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
}

type Auth struct {
	Token string `json:"token"`
}

type Person struct {
	Name Name `json:"name"`
	Auth Auth `json:"auth"`
	Age  int  `json:"age,omitempty"`
}

type User struct {
	LastName string `json:"last_name"`
	UserID   int    `json:"user_id"`
}
