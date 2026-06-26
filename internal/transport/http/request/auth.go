package request

type RegisterRequest struct {
	Username string  `json:"username" binding:"required,min=1,max=64"`
	Nickname *string `json:"nickname" binding:"omitempty,max=64"`
	Email    *string `json:"email" binding:"omitempty,email,max=255"`
	Avatar   *string `json:"avatar" binding:"omitempty,max=512"`
	Password string  `json:"password" binding:"required,min=6,max=255"`
}

type LoginRequest struct {
	Login    string `json:"login" binding:"required,min=1,max=255"`
	Password string `json:"password" binding:"required,min=6,max=255"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ProfileRequest struct {
	Nickname *string `json:"nickname" binding:"omitempty,max=64"`
	Email    *string `json:"email" binding:"omitempty,email,max=255"`
}

type PasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=255"`
}
