package rbac

import (
	"billionmail-core/api/rbac/v1"
	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/public"
	service "billionmail-core/internal/service/rbac"
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/util/gconv"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/crypto/bcrypt"
)

var signupUsernameRegexp = regexp.MustCompile(`^[A-Za-z0-9_]{4,32}$`)

func grantSafePathPass(ctx context.Context) {
	if request := g.RequestFromCtx(ctx); request != nil {
		_ = request.Session.Set("safe_path_pass", true)
	}
}

func normalizeSignupReq(req *v1.SignupReq) {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
}

func validateSignupReq(req *v1.SignupReq) error {
	if !signupUsernameRegexp.MatchString(req.Username) {
		return fmt.Errorf("Username must be 4-32 characters and contain only letters, numbers, or underscores")
	}

	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email {
		return fmt.Errorf("Email must be valid")
	}

	if len(req.Password) < 8 {
		return fmt.Errorf("Password must be at least 8 characters")
	}

	if req.Password != req.ConfirmPassword {
		return fmt.Errorf("Passwords do not match")
	}

	return nil
}

// Login handles user login
func (c *ControllerV1) Login(ctx context.Context, req *v1.LoginReq) (res *v1.LoginRes, err error) {
	res = &v1.LoginRes{}

	maxRetries := 5
	blockTime := 300

	// Default validate status
	validateSuccess := true

	// Login success flag in current request
	loginSuccessFlag := false

	clientIp := g.RequestFromCtx(ctx).GetClientIp()

	cacheKey := fmt.Sprintf("USER_LOGIN_RETRIES:%s", clientIp)

	loginRetries, mustValidateCode := public.GetCache(cacheKey).(int)

	// End of request, check if login was successful
	defer func() {
		// Validate failure
		if !validateSuccess {
			return
		}

		// Login success
		if loginSuccessFlag {
			public.RemoveCache(cacheKey)
			return
		}

		// Increment login retries
		public.SetCache(cacheKey, loginRetries+1, 300)
	}()

	if loginRetries >= maxRetries {
		k := "USER_LOGIN_RETRIES_RELEASE_TIME:" + clientIp
		releaseTime, blocked := public.GetCache(k).(int64)
		if !blocked {
			releaseTime = time.Now().Unix() + int64(blockTime)
			public.SetCache(k, releaseTime, blockTime)
		}

		err = fmt.Errorf("Login failed too many times, please try again after %d seconds", releaseTime-time.Now().Unix())
		return
	}

	// Check if validation code is required
	if mustValidateCode {
		validateSuccess = false

		if req.ValidateCodeId == "" || req.ValidateCode == "" {
			err = fmt.Errorf("Validation code ID and code cannot be empty")
			return
		}

		if !service.VerifyCaptcha(req.ValidateCodeId, req.ValidateCode) {
			err = fmt.Errorf("Invalid validation code")
			return
		}

		validateSuccess = true
	}

	// Verify username and password
	account, err := service.Account().Login(ctx, req.Username, req.Password)
	if err != nil {
		err = fmt.Errorf("Invalid username or password")
		return
	}

	// Get account roles
	roles, err := service.Account().GetAccountRoles(ctx, account.AccountId)
	if err != nil {
		err = fmt.Errorf("Failed to get account roles")
		return
	}

	// Convert roles to role names
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.RoleName)
	}

	// Generate JWT token
	token, _, err := service.JWT().GenerateToken(account.AccountId, account.Username, roleNames)
	if err != nil {
		res.SetError(gerror.New("Failed to generate token"))
		return
	}

	// Generate refresh token
	refreshToken, err := service.JWT().GenerateRefreshToken(account.AccountId, account.Username)
	if err != nil {
		res.SetError(gerror.New("Failed to generate refresh token"))
		return
	}

	// Prepare response
	res.Success = true
	res.Code = 0
	res.Msg = "Login successful"
	res.Data.Token = token
	res.Data.RefreshToken = refreshToken
	res.Data.TTL = gconv.Int64(service.JWT().AccessExpiry.Seconds())

	// Set account basic information
	res.Data.AccountInfo.Id = account.AccountId
	res.Data.AccountInfo.Username = account.Username
	res.Data.AccountInfo.Email = account.Email
	res.Data.AccountInfo.Status = account.Status
	res.Data.AccountInfo.Lang = account.Language

	loginSuccessFlag = true
	grantSafePathPass(ctx)

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Login,
		Log:  "The user:" + req.Username + " login was successful",
		Data: res.Data,
	})
	return
}

// Signup creates a public account and signs it in.
func (c *ControllerV1) Signup(ctx context.Context, req *v1.SignupReq) (res *v1.SignupRes, err error) {
	res = &v1.SignupRes{}
	normalizeSignupReq(req)

	if validateErr := validateSignupReq(req); validateErr != nil {
		res.SetError(validateErr)
		return
	}

	language := "en"
	status := 1
	roleNames := []string{"admin"}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		res.SetError(gerror.New("Failed to hash password"))
		return
	}

	var accountId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		usernameCount, err := tx.Model("account").Where("username = ?", req.Username).Count()
		if err != nil {
			return fmt.Errorf("Failed to check username")
		}
		if usernameCount > 0 {
			return fmt.Errorf("Username already exists")
		}

		emailCount, err := tx.Model("account").Where("email = ?", req.Email).Count()
		if err != nil {
			return fmt.Errorf("Failed to check email")
		}
		if emailCount > 0 {
			return fmt.Errorf("Email already exists")
		}

		now := time.Now().Unix()
		adminRoleId := int64(0)
		adminRoleIdVal, err := tx.Model("role").Where("role_name = ?", "admin").Value("role_id")
		if err != nil {
			return fmt.Errorf("Failed to check admin role")
		}
		if adminRoleIdVal == nil {
			result, err := tx.Model("role").Data(g.Map{
				"role_name":   "admin",
				"description": "System administrator with full access",
				"status":      1,
				"create_time": now,
				"update_time": now,
			}).Insert()
			if err != nil {
				return fmt.Errorf("Failed to create admin role")
			}
			adminRoleId, err = result.LastInsertId()
			if err != nil {
				return fmt.Errorf("Failed to read admin role ID")
			}
		} else {
			adminRoleId = adminRoleIdVal.Int64()
		}

		result, err := tx.Model("account").Data(g.Map{
			"username":    req.Username,
			"password":    string(passwordHash),
			"email":       req.Email,
			"status":      status,
			"language":    language,
			"create_time": now,
			"update_time": now,
		}).Insert()
		if err != nil {
			return fmt.Errorf("Failed to create account")
		}

		accountId, err = result.LastInsertId()
		if err != nil {
			return fmt.Errorf("Failed to read account ID")
		}

		_, err = tx.Model("account_role").InsertIgnore(g.Map{
			"account_id":  accountId,
			"role_id":     adminRoleId,
			"create_time": now,
		})
		if err != nil {
			return fmt.Errorf("Failed to assign admin role")
		}

		return nil
	})
	if err != nil {
		res.SetError(err)
		return
	}

	token, _, err := service.JWT().GenerateToken(accountId, req.Username, roleNames)
	if err != nil {
		res.SetError(gerror.New("Failed to generate token"))
		return
	}

	refreshToken, err := service.JWT().GenerateRefreshToken(accountId, req.Username)
	if err != nil {
		res.SetError(gerror.New("Failed to generate refresh token"))
		return
	}

	res.Success = true
	res.Code = 0
	res.Msg = "Signup successful"
	res.Data.Token = token
	res.Data.RefreshToken = refreshToken
	res.Data.TTL = gconv.Int64(service.JWT().AccessExpiry.Seconds())
	res.Data.AccountInfo.Id = accountId
	res.Data.AccountInfo.Username = req.Username
	res.Data.AccountInfo.Email = req.Email
	res.Data.AccountInfo.Status = status
	res.Data.AccountInfo.Lang = language
	grantSafePathPass(ctx)

	_ = public.WriteLog(ctx, public.LogParams{
		Type: consts.LOGTYPE.Login,
		Log:  "The user:" + req.Username + " signup was successful",
		Data: res.Data,
	})

	return
}

// Logout handles user logout
func (c *ControllerV1) Logout(ctx context.Context, req *v1.LogoutReq) (res *v1.LogoutRes, err error) {
	res = &v1.LogoutRes{}

	// Parse the token from the request
	claims, err := service.JWT().ParseToken(req.Authorization)
	if err != nil {
		res.SetError(gerror.New("Invalid or expired refresh token"))
		return
	}

	// Add token to blacklist
	err = service.JWT().InvalidateToken(claims)
	if err != nil {
		err = fmt.Errorf("Logout failed: %w", err)
		return
	}

	// Destroy the session
	_ = g.RequestFromCtx(ctx).Session.RemoveAll()

	// reset the safe path pass
	_ = g.RequestFromCtx(ctx).Session.Set("safe_path_pass", true)

	res.Success = true
	res.Code = 0
	res.Msg = "Logout successful"

	return
}

// RefreshToken handles token refresh
func (c *ControllerV1) RefreshToken(ctx context.Context, req *v1.RefreshTokenReq) (res *v1.RefreshTokenRes, err error) {
	res = &v1.RefreshTokenRes{}

	// Verify refresh token
	claims, err := service.JWT().ParseToken(req.RefreshToken)
	if err != nil {
		res.SetError(gerror.New("Invalid or expired refresh token"))
		return
	}

	// Validate token type — only accept refresh tokens
	if claims.Subject != "refresh_token" {
		res.SetError(gerror.New("Invalid token type"))
		return
	}

	// Get account roles from database
	roles, roleErr := service.Account().GetAccountRoles(ctx, claims.AccountId)
	if roleErr != nil {
		res.SetError(gerror.New("Failed to get account roles"))
		return
	}
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.RoleName)
	}

	// Generate new JWT token
	token, _, err := service.JWT().GenerateToken(claims.AccountId, claims.Username, roleNames)
	if err != nil {
		res.SetError(gerror.New("Failed to generate token"))
		return
	}

	// Generate new refresh token
	refreshToken, err := service.JWT().GenerateRefreshToken(claims.AccountId, claims.Username)
	if err != nil {
		res.SetError(gerror.New("Failed to generate refresh token"))
		return
	}

	// Prepare response
	res.Success = true
	res.Code = 0
	res.Msg = "Token refreshed successfully"
	res.Data.Token = token
	res.Data.RefreshToken = refreshToken
	res.Data.TTL = gconv.Int64(service.JWT().AccessExpiry.Seconds())
	grantSafePathPass(ctx)

	return
}

//// CurrentUser retrieves the current logged-in user information
//func (c *ControllerV1) CurrentUser(ctx context.Context, req *v1.CurrentUserReq) (res *v1.CurrentUserRes, err error) {
//	res = &v1.CurrentUserRes{}
//
//	// Get account ID from context
//	accountId := service.GetCurrentAccountId(ctx)
//	if accountId == 0 {
//		err = gerror.New("Unauthorized")
//		return
//	}
//
//	// Get account details
//	account, err := service.Account().GetById(ctx, accountId)
//	if err != nil {
//		err = gerror.New("Failed to get account details")
//		return
//	}
//
//	// Get account roles
//	roles, err := service.Account().GetAccountRoles(ctx, accountId)
//	if err != nil {
//		err = gerror.New("Failed to get account roles")
//		return
//	}
//
//	// Get account permissions
//	permissions, err := service.Account().GetAccountPermissions(ctx, accountId)
//	if err != nil {
//		err = gerror.New("Failed to get account permissions")
//		return
//	}
//
//	// Prepare response
//	res.Success = true
//	res.Code = 0
//	res.Msg = "Retrieved successfully"
//
//	// Set account information
//	res.Data.Account.Id = account.AccountId
//	res.Data.Account.Username = account.Username
//	res.Data.Account.Email = account.Email
//	res.Data.Account.Status = account.Status
//	res.Data.Account.Lang = account.Language
//
//	// Set roles
//	res.Data.Roles = make([]string, 0, len(roles))
//	for _, role := range roles {
//		res.Data.Roles = append(res.Data.Roles, role.RoleName)
//	}
//
//	// Set permissions
//	res.Data.Permissions = make([]string, 0, len(permissions))
//	for _, perm := range permissions {
//		permStr := perm.Module + ":" + perm.Action + ":" + perm.Resource
//		res.Data.Permissions = append(res.Data.Permissions, permStr)
//	}
//
//	return
//}
