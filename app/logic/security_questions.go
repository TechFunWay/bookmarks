package logic

import (
	"database/sql"
	"fmt"
	"strings"
)

// SecurityQuestionsRequest 设置安全问题请求
type SecurityQuestionsRequest struct {
	Question1 string `json:"question1"`
	Answer1   string `json:"answer1"`
	Question2 string `json:"question2"`
	Answer2   string `json:"answer2"`
	Question3 string `json:"question3"`
	Answer3   string `json:"answer3"`
}

// VerifySecurityQuestionsRequest 验证安全问题请求
type VerifySecurityQuestionsRequest struct {
	Username    string `json:"username"`
	Answer1     string `json:"answer1"`
	Answer2     string `json:"answer2"`
	Answer3     string `json:"answer3"`
	NewPassword string `json:"new_password"`
}

// SecurityQuestions 安全问题管理器
type SecurityQuestions struct {
	db *sql.DB
}

// NewSecurityQuestions 创建安全问题管理器
func NewSecurityQuestions(db *sql.DB) *SecurityQuestions {
	return &SecurityQuestions{db: db}
}

// SetSecurityQuestions 设置/更新用户的安全问题
func (sq *SecurityQuestions) SetSecurityQuestions(userID int64, req *SecurityQuestionsRequest) error {
	// 去除前后空格
	req.Question1 = strings.TrimSpace(req.Question1)
	req.Answer1 = strings.TrimSpace(req.Answer1)
	req.Question2 = strings.TrimSpace(req.Question2)
	req.Answer2 = strings.TrimSpace(req.Answer2)
	req.Question3 = strings.TrimSpace(req.Question3)
	req.Answer3 = strings.TrimSpace(req.Answer3)

	// 验证字段不为空
	if req.Question1 == "" || req.Answer1 == "" ||
		req.Question2 == "" || req.Answer2 == "" ||
		req.Question3 == "" || req.Answer3 == "" {
		return fmt.Errorf("所有问题和答案不能为空")
	}

	// 检查是否已设置过安全问题
	var count int
	err := sq.db.QueryRow("SELECT COUNT(*) FROM security_questions WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return fmt.Errorf("查询安全问题失败: %w", err)
	}

	if count > 0 {
		// 更新现有的安全问题
		_, err = sq.db.Exec(`
			UPDATE security_questions
			SET question1 = ?, answer1 = ?, question2 = ?, answer2 = ?, question3 = ?, answer3 = ?, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = ?`,
			req.Question1, req.Answer1, req.Question2, req.Answer2, req.Question3, req.Answer3, userID)
	} else {
		// 插入新的安全问题
		_, err = sq.db.Exec(`
			INSERT INTO security_questions (user_id, question1, answer1, question2, answer2, question3, answer3)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID, req.Question1, req.Answer1, req.Question2, req.Answer2, req.Question3, req.Answer3)
	}

	if err != nil {
		return fmt.Errorf("保存安全问题失败: %w", err)
	}

	return nil
}

// GetSecurityQuestionsResponse 获取安全问题响应
type GetSecurityQuestionsResponse struct {
	HasSecurityQuestions bool              `json:"has_security_questions"`
	Questions            map[string]string `json:"questions"`
	AnswerLengths        map[string]int    `json:"answer_lengths"`
}

// GetSecurityQuestions 获取用户的安全问题（仅返回问题，不返回答案）
func (sq *SecurityQuestions) GetSecurityQuestions(userID int64) (*GetSecurityQuestionsResponse, error) {
	var question1, question2, question3 *string
	var answer1, answer2, answer3 *string
	var count int

	// 先检查是否存在安全问题
	err := sq.db.QueryRow("SELECT COUNT(*) FROM security_questions WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("查询安全问题失败: %w", err)
	}

	if count > 0 {
		err = sq.db.QueryRow(`
			SELECT question1, question2, question3, answer1, answer2, answer3
			FROM security_questions
			WHERE user_id = ?`,
			userID).Scan(&question1, &question2, &question3, &answer1, &answer2, &answer3)
		if err != nil {
			return nil, fmt.Errorf("获取安全问题失败: %w", err)
		}
	}

	response := &GetSecurityQuestionsResponse{
		HasSecurityQuestions: count > 0,
		Questions: map[string]string{
			"question1": "",
			"question2": "",
			"question3": "",
		},
		AnswerLengths: map[string]int{
			"answer1": 0,
			"answer2": 0,
			"answer3": 0,
		},
	}

	if question1 != nil {
		response.Questions["question1"] = *question1
	}
	if question2 != nil {
		response.Questions["question2"] = *question2
	}
	if question3 != nil {
		response.Questions["question3"] = *question3
	}

	if answer1 != nil {
		response.AnswerLengths["answer1"] = len(*answer1)
	}
	if answer2 != nil {
		response.AnswerLengths["answer2"] = len(*answer2)
	}
	if answer3 != nil {
		response.AnswerLengths["answer3"] = len(*answer3)
	}

	return response, nil
}

// GetUserIDByUsername 根据用户名获取用户ID
func (sq *SecurityQuestions) GetUserIDByUsername(username string) (int64, error) {
	var userID int64
	err := sq.db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("用户不存在")
		}
		return 0, fmt.Errorf("查询用户失败: %w", err)
	}
	return userID, nil
}

// GetSecurityQuestionsForReset 获取用户的安全问题（用于重置密码，需要用户名）
func (sq *SecurityQuestions) GetSecurityQuestionsForReset(username string) (*map[string]string, error) {
	// 获取用户ID
	userID, err := sq.GetUserIDByUsername(username)
	if err != nil {
		return nil, err
	}

	// 检查是否设置了安全问题
	var count int
	err = sq.db.QueryRow("SELECT COUNT(*) FROM security_questions WHERE user_id = ?", userID).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("查询安全问题失败: %w", err)
	}

	if count == 0 {
		return nil, fmt.Errorf("该用户未设置安全问题，无法通过此方式重置密码")
	}

	// 获取安全问题（不返回答案）
	var question1, question2, question3 string
	err = sq.db.QueryRow(`
		SELECT question1, question2, question3
		FROM security_questions
		WHERE user_id = ?`,
		userID).Scan(&question1, &question2, &question3)
	if err != nil {
		return nil, fmt.Errorf("获取安全问题失败: %w", err)
	}

	questions := map[string]string{
		"question1": question1,
		"question2": question2,
		"question3": question3,
	}

	return &questions, nil
}

// VerifyAndResetPassword 验证安全问题并重置密码
func (sq *SecurityQuestions) VerifyAndResetPassword(req *VerifySecurityQuestionsRequest) error {
	// 去除前后空格
	req.Username = strings.TrimSpace(req.Username)
	req.Answer1 = strings.TrimSpace(req.Answer1)
	req.Answer2 = strings.TrimSpace(req.Answer2)
	req.Answer3 = strings.TrimSpace(req.Answer3)
	req.NewPassword = strings.TrimSpace(req.NewPassword)

	// 验证字段
	if req.Username == "" || req.NewPassword == "" {
		return fmt.Errorf("用户名和新密码不能为空")
	}

	if len(req.NewPassword) < 6 {
		return fmt.Errorf("新密码长度不能少于6位")
	}

	// 获取用户ID
	userID, err := sq.GetUserIDByUsername(req.Username)
	if err != nil {
		return err
	}

	// 验证安全问题
	var dbAnswer1, dbAnswer2, dbAnswer3 string
	err = sq.db.QueryRow(`
		SELECT answer1, answer2, answer3
		FROM security_questions
		WHERE user_id = ?`,
		userID).Scan(&dbAnswer1, &dbAnswer2, &dbAnswer3)
	if err != nil {
		return fmt.Errorf("查询安全问题失败: %w", err)
	}

	// 验证答案（不区分大小写）
	if !strings.EqualFold(req.Answer1, dbAnswer1) ||
		!strings.EqualFold(req.Answer2, dbAnswer2) ||
		!strings.EqualFold(req.Answer3, dbAnswer3) {
		return fmt.Errorf("安全问题答案错误")
	}

	return nil
}

// UpdatePassword 更新用户密码
func (sq *SecurityQuestions) UpdatePassword(username, hashedPassword string) error {
	// 获取用户ID
	userID, err := sq.GetUserIDByUsername(username)
	if err != nil {
		return err
	}

	_, err = sq.db.Exec("UPDATE users SET password = ? WHERE id = ?", hashedPassword, userID)
	if err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}

	return nil
}
