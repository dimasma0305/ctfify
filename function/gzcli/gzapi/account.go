package gzapi

func (cs *GZAPI) Login() error {
	if err := cs.post("/api/account/login", cs.Creds, nil); err != nil {
		return err
	}
	return nil
}

type RegisterForm struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (cs *GZAPI) Register(registerForm *RegisterForm) error {
	if err := cs.post("/api/account/register", registerForm, nil); err != nil {
		return err
	}
	return nil
}

func (cs *GZAPI) Logout() error {
	if err := cs.post("/api/account/logout", nil, nil); err != nil {
		return err
	}
	return nil
}
