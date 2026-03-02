package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// TwoFactorSetupResponse contains the TOTP secret and QR code
type TwoFactorSetupResponse struct {
	Secret    string   `json:"secret"`
	QRCodeURL string   `json:"qr_code_url"`
	QRCodeSVG string   `json:"qr_code_svg"`
}

// TwoFactorEnableRequest contains the verification code
type TwoFactorEnableRequest struct {
	Code string `json:"code"`
}

// TwoFactorEnableResponse contains backup codes
type TwoFactorEnableResponse struct {
	BackupCodes []string `json:"backup_codes"`
}

// BackupCode represents a single backup code with its hash
type BackupCode struct {
	Code string `json:"code"`
	Hash string `json:"hash"`
}

// HandleChangePassword allows users to change their password
func (s *Server) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate new password
	if len(req.NewPassword) < 8 {
		http.Error(w, "New password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Get current password hash
	var currentHash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = $1", userID).Scan(&currentHash)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Update password
	_, err = s.db.Exec("UPDATE users SET password_hash = $1, password_changed_at = $2 WHERE id = $3",
		string(newHash), time.Now(), userID)
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Password changed successfully"})
}

// Handle2FASetup initiates 2FA setup by generating a secret and QR code
func (s *Server) Handle2FASetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user email
	var email string
	err := s.db.QueryRow("SELECT email FROM users WHERE id = $1", userID).Scan(&email)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "TaskAI",
		AccountName: email,
		SecretSize:  32,
	})
	if err != nil {
		http.Error(w, "Failed to generate TOTP key", http.StatusInternalServerError)
		return
	}

	// Store the secret (but don't enable 2FA yet)
	_, err = s.db.Exec("UPDATE users SET totp_secret = $1 WHERE id = $2", key.Secret(), userID)
	if err != nil {
		http.Error(w, "Failed to save TOTP secret", http.StatusInternalServerError)
		return
	}

	// Generate QR code as PNG, then base64 encode it
	qrCode, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
		return
	}
	qrCodeDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrCode)

	response := TwoFactorSetupResponse{
		Secret:    key.Secret(),
		QRCodeURL: qrCodeDataURL,
		QRCodeSVG: key.URL(), // Provide the URL for manual entry
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Handle2FAEnable verifies the TOTP code and enables 2FA
func (s *Server) Handle2FAEnable(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req TwoFactorEnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Get TOTP secret
	var secret string
	err := s.db.QueryRow("SELECT totp_secret FROM users WHERE id = $1", userID).Scan(&secret)
	if err != nil || secret == "" {
		http.Error(w, "2FA setup not initiated", http.StatusBadRequest)
		return
	}

	// Verify TOTP code
	valid := totp.Validate(req.Code, secret)
	if !valid {
		http.Error(w, "Invalid verification code", http.StatusUnauthorized)
		return
	}

	// Generate backup codes
	backupCodes, err := generateBackupCodes(10)
	if err != nil {
		http.Error(w, "Failed to generate backup codes", http.StatusInternalServerError)
		return
	}

	// Hash backup codes for storage
	hashedCodes := make([]BackupCode, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash backup codes", http.StatusInternalServerError)
			return
		}
		hashedCodes[i] = BackupCode{
			Code: code,
			Hash: string(hash),
		}
	}

	// Store hashed backup codes as JSON
	backupCodesJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		http.Error(w, "Failed to serialize backup codes", http.StatusInternalServerError)
		return
	}

	// Enable 2FA
	_, err = s.db.Exec("UPDATE users SET totp_enabled = 1, backup_codes = $1 WHERE id = $2",
		string(backupCodesJSON), userID)
	if err != nil {
		http.Error(w, "Failed to enable 2FA", http.StatusInternalServerError)
		return
	}

	response := TwoFactorEnableResponse{
		BackupCodes: backupCodes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Handle2FADisable disables 2FA for the user
func (s *Server) Handle2FADisable(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Verify password before disabling 2FA
	var passwordHash string
	err := s.db.QueryRow("SELECT password_hash FROM users WHERE id = $1", userID).Scan(&passwordHash)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		http.Error(w, "Incorrect password", http.StatusUnauthorized)
		return
	}

	// Disable 2FA
	_, err = s.db.Exec("UPDATE users SET totp_enabled = 0, totp_secret = NULL, backup_codes = NULL WHERE id = $1", userID)
	if err != nil {
		http.Error(w, "Failed to disable 2FA", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "2FA disabled successfully"})
}

// Handle2FAStatus returns the current 2FA status
func (s *Server) Handle2FAStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var totpEnabled int
	err := s.db.QueryRow("SELECT totp_enabled FROM users WHERE id = $1", userID).Scan(&totpEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"enabled": totpEnabled == 1,
	})
}

// generateBackupCodes generates n random backup codes
func generateBackupCodes(n int) ([]string, error) {
	codes := make([]string, n)
	for i := 0; i < n; i++ {
		// Generate 8 random bytes
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		// Format as XXXX-XXXX
		code := fmt.Sprintf("%04X-%04X",
			uint16(b[0])<<8|uint16(b[1]),
			uint16(b[2])<<8|uint16(b[3]))
		codes[i] = code
	}
	return codes, nil
}
