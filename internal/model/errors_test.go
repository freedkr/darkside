package model

import (
	"errors"
	"strings"
	"testing"
)

func TestNewParseError(t *testing.T) {
	err := NewParseError(10, 5, "test content", "test field", "parse failed")
	
	if err.Row != 10 {
		t.Errorf("Expected row 10, got %d", err.Row)
	}
	if err.Column != 5 {
		t.Errorf("Expected column 5, got %d", err.Column)
	}
	if err.Content != "test content" {
		t.Errorf("Expected content 'test content', got '%s'", err.Content)
	}
	if err.Field != "test field" {
		t.Errorf("Expected field 'test field', got '%s'", err.Field)
	}
	if err.Message != "parse failed" {
		t.Errorf("Expected message 'parse failed', got '%s'", err.Message)
	}
	if err.Code != ErrCodeParseError {
		t.Errorf("Expected code %s, got %s", ErrCodeParseError, err.Code)
	}
}

func TestParseError_Error(t *testing.T) {
	err := NewParseError(10, 5, "test content", "name", "invalid format")
	
	errorMsg := err.Error()
	expectedParts := []string{"行10列5解析失败", "invalid format", "test content", "name"}
	
	for _, part := range expectedParts {
		if !strings.Contains(errorMsg, part) {
			t.Errorf("Error message should contain '%s', got '%s'", part, errorMsg)
		}
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("code", "invalid-code", "format", "invalid code format")
	
	if err.Field != "code" {
		t.Errorf("Expected field 'code', got '%s'", err.Field)
	}
	if err.Value != "invalid-code" {
		t.Errorf("Expected value 'invalid-code', got '%v'", err.Value)
	}
	if err.Constraint != "format" {
		t.Errorf("Expected constraint 'format', got '%s'", err.Constraint)
	}
	if err.Message != "invalid code format" {
		t.Errorf("Expected message 'invalid code format', got '%s'", err.Message)
	}
	if err.Code != ErrCodeValidation {
		t.Errorf("Expected code %s, got %s", ErrCodeValidation, err.Code)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := NewValidationError("name", "", "required", "name is required")
	
	errorMsg := err.Error()
	expectedParts := []string{"字段'name'验证失败", "name is required", "required"}
	
	for _, part := range expectedParts {
		if !strings.Contains(errorMsg, part) {
			t.Errorf("Error message should contain '%s', got '%s'", part, errorMsg)
		}
	}
}

func TestNewSystemError(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewSystemError("parser", "parse", "system error occurred", cause)
	
	if err.Component != "parser" {
		t.Errorf("Expected component 'parser', got '%s'", err.Component)
	}
	if err.Operation != "parse" {
		t.Errorf("Expected operation 'parse', got '%s'", err.Operation)
	}
	if err.Message != "system error occurred" {
		t.Errorf("Expected message 'system error occurred', got '%s'", err.Message)
	}
	if err.Cause != cause {
		t.Errorf("Expected cause to match")
	}
	if err.Code != ErrCodeInternal {
		t.Errorf("Expected code %s, got %s", ErrCodeInternal, err.Code)
	}
}

func TestSystemError_Error(t *testing.T) {
	cause := errors.New("database connection failed")
	err := NewSystemError("database", "connect", "connection error", cause)
	
	errorMsg := err.Error()
	expectedParts := []string{"database.connect失败", "connection error", "database connection failed"}
	
	for _, part := range expectedParts {
		if !strings.Contains(errorMsg, part) {
			t.Errorf("Error message should contain '%s', got '%s'", part, errorMsg)
		}
	}
}

func TestSystemError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	err := NewSystemError("component", "operation", "message", cause)
	
	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Expected unwrapped error to be the original cause")
	}
}

func TestNewFileError(t *testing.T) {
	cause := errors.New("file not found")
	err := NewFileError(ErrCodeFileNotFound, "/path/to/file.txt", "open", "file error", cause)
	
	if err.FilePath != "/path/to/file.txt" {
		t.Errorf("Expected file path '/path/to/file.txt', got '%s'", err.FilePath)
	}
	if err.Operation != "open" {
		t.Errorf("Expected operation 'open', got '%s'", err.Operation)
	}
	if err.Message != "file error" {
		t.Errorf("Expected message 'file error', got '%s'", err.Message)
	}
	if err.Cause != cause {
		t.Errorf("Expected cause to match")
	}
	if err.Code != ErrCodeFileNotFound {
		t.Errorf("Expected code %s, got %s", ErrCodeFileNotFound, err.Code)
	}
}

func TestFileError_Error(t *testing.T) {
	cause := errors.New("permission denied")
	err := NewFileError(ErrCodeFileReadError, "/tmp/test.txt", "read", "read failed", cause)
	
	errorMsg := err.Error()
	expectedParts := []string{"文件操作失败", "read('/tmp/test.txt')", "read failed", "permission denied"}
	
	for _, part := range expectedParts {
		if !strings.Contains(errorMsg, part) {
			t.Errorf("Error message should contain '%s', got '%s'", part, errorMsg)
		}
	}
}

func TestNewHierarchyError(t *testing.T) {
	err := NewHierarchyError("1-01-01", "1-01", "missing_parent", "parent not found", 2)
	
	if err.Code1 != "1-01-01" {
		t.Errorf("Expected code1 '1-01-01', got '%s'", err.Code1)
	}
	if err.Code2 != "1-01" {
		t.Errorf("Expected code2 '1-01', got '%s'", err.Code2)
	}
	if err.Operation != "missing_parent" {
		t.Errorf("Expected operation 'missing_parent', got '%s'", err.Operation)
	}
	if err.Message != "parent not found" {
		t.Errorf("Expected message 'parent not found', got '%s'", err.Message)
	}
	if err.Level != 2 {
		t.Errorf("Expected level 2, got %d", err.Level)
	}
	if err.BaseError.Code != ErrCodeHierarchy {
		t.Errorf("Expected code %s, got %s", ErrCodeHierarchy, err.BaseError.Code)
	}
}

func TestHierarchyError_Error(t *testing.T) {
	err := NewHierarchyError("1-01-01", "1-01", "build", "parent relationship error", 3)
	
	errorMsg := err.Error()
	expectedParts := []string{"层级结构错误", "build('1-01-01' -> '1-01')", "parent relationship error", "层级: 3"}
	
	for _, part := range expectedParts {
		if !strings.Contains(errorMsg, part) {
			t.Errorf("Error message should contain '%s', got '%s'", part, errorMsg)
		}
	}
}

func TestErrorList_Add(t *testing.T) {
	errorList := NewErrorList()
	
	if errorList.Count() != 0 {
		t.Errorf("Expected empty error list, got %d errors", errorList.Count())
	}
	
	err1 := NewValidationError("field1", "value1", "required", "field1 is required")
	err2 := NewParseError(1, 2, "content", "field2", "parse error")
	
	errorList.Add(err1)
	if errorList.Count() != 1 {
		t.Errorf("Expected 1 error after adding, got %d", errorList.Count())
	}
	
	errorList.Add(err2)
	if errorList.Count() != 2 {
		t.Errorf("Expected 2 errors after adding, got %d", errorList.Count())
	}
	
	// 添加nil错误应该被忽略
	errorList.Add(nil)
	if errorList.Count() != 2 {
		t.Errorf("Expected 2 errors after adding nil, got %d", errorList.Count())
	}
}

func TestErrorList_HasError(t *testing.T) {
	errorList := NewErrorList()
	
	if errorList.HasError() {
		t.Error("Expected no errors in empty list")
	}
	
	err := NewValidationError("field", "value", "constraint", "message")
	errorList.Add(err)
	
	if !errorList.HasError() {
		t.Error("Expected errors after adding an error")
	}
}

func TestErrorList_Error(t *testing.T) {
	errorList := NewErrorList()
	
	// 空列表
	if errorList.Error() != "" {
		t.Errorf("Expected empty string for empty error list, got '%s'", errorList.Error())
	}
	
	// 单个错误
	err1 := NewValidationError("field1", "value1", "required", "field1 is required")
	errorList.Add(err1)
	
	errorMsg := errorList.Error()
	if errorMsg != err1.Error() {
		t.Errorf("Expected single error message, got '%s'", errorMsg)
	}
	
	// 多个错误
	err2 := NewParseError(1, 2, "content", "field2", "parse error")
	errorList.Add(err2)
	
	errorMsg = errorList.Error()
	if !strings.Contains(errorMsg, "发生了2个错误") {
		t.Errorf("Expected multi-error message format, got '%s'", errorMsg)
	}
	if !strings.Contains(errorMsg, "field1 is required") {
		t.Errorf("Expected first error message in combined error, got '%s'", errorMsg)
	}
	if !strings.Contains(errorMsg, "parse error") {
		t.Errorf("Expected second error message in combined error, got '%s'", errorMsg)
	}
}

func TestErrorList_GetByType(t *testing.T) {
	errorList := NewErrorList()
	
	validationErr := NewValidationError("field", "value", "constraint", "validation error")
	parseErr := NewParseError(1, 2, "content", "field", "parse error")
	systemErr := NewSystemError("component", "operation", "system error", nil)
	
	errorList.Add(validationErr)
	errorList.Add(parseErr)
	errorList.Add(systemErr)
	
	// 获取验证错误
	validationErrors := errorList.GetByType(ErrCodeValidation)
	if len(validationErrors) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(validationErrors))
	}
	
	// 获取解析错误
	parseErrors := errorList.GetByType(ErrCodeParseError)
	if len(parseErrors) != 1 {
		t.Errorf("Expected 1 parse error, got %d", len(parseErrors))
	}
	
	// 获取不存在的错误类型
	fileErrors := errorList.GetByType(ErrCodeFileNotFound)
	if len(fileErrors) != 0 {
		t.Errorf("Expected 0 file errors, got %d", len(fileErrors))
	}
}

func TestIsErrorType(t *testing.T) {
	validationErr := NewValidationError("field", "value", "constraint", "validation error")
	parseErr := NewParseError(1, 2, "content", "field", "parse error")
	standardErr := errors.New("standard error")
	
	// 测试匹配
	if !IsErrorType(validationErr, ErrCodeValidation) {
		t.Error("Expected validation error to match ErrCodeValidation")
	}
	
	if !IsErrorType(parseErr, ErrCodeParseError) {
		t.Error("Expected parse error to match ErrCodeParseError")
	}
	
	// 测试不匹配
	if IsErrorType(validationErr, ErrCodeParseError) {
		t.Error("Expected validation error not to match ErrCodeParseError")
	}
	
	if IsErrorType(parseErr, ErrCodeValidation) {
		t.Error("Expected parse error not to match ErrCodeValidation")
	}
	
	// 测试标准错误
	if IsErrorType(standardErr, ErrCodeValidation) {
		t.Error("Expected standard error not to match any custom error type")
	}
	
	// 测试nil错误
	if IsErrorType(nil, ErrCodeValidation) {
		t.Error("Expected nil error not to match any error type")
	}
}

func TestBaseError_WithStackTrace(t *testing.T) {
	baseErr := &BaseError{
		Code:    ErrCodeInternal,
		Message: "test error",
	}
	
	// 初始没有堆栈跟踪
	if baseErr.StackTrace != "" {
		t.Error("Expected empty stack trace initially")
	}
	
	// 添加堆栈跟踪
	result := baseErr.WithStackTrace()
	
	// 应该返回相同的实例
	if result != baseErr {
		t.Error("Expected WithStackTrace to return the same instance")
	}
	
	// 应该添加堆栈跟踪
	if baseErr.StackTrace == "" {
		t.Error("Expected stack trace to be added")
	}
	
	// 再次调用不应该覆盖现有的堆栈跟踪
	oldStackTrace := baseErr.StackTrace
	baseErr.WithStackTrace()
	
	if baseErr.StackTrace != oldStackTrace {
		t.Error("Expected stack trace not to be overwritten")
	}
}