package gzapi

import "fmt"

type User struct {
	Id       string `json:"id"`
	UserName string `json:"username"`
	Bio      string `json:"bio"`
	Captain  bool   `json:"captain"`
	API      *GZAPI `json:"-"`
}

func (user *User) Delete() error {
	if err := user.API.delete(fmt.Sprintf("/api/admin/users/%s", user.Id), nil); err != nil {
		return err
	}
	return nil
}

func (api *GZAPI) Users() ([]*User, error) {
	var users struct {
		Data []*User `json:"data"`
	}
	if err := api.get("/api/admin/users", &users); err != nil {
		return nil, err
	}
	for t := range users.Data {
		users.Data[t].API = api
	}
	return users.Data, nil
}

func (api *GZAPI) JoinGame(gameId int, joinModel *GameJoinModel) error {
	return api.post(fmt.Sprintf("/api/game/%d", gameId), joinModel, nil)
}
