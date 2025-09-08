// Package model 定义自定义错误类型
package model

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// ErrorCode 错误代码类型
type ErrorCode string

// 预定义错误代码
const (
	// 通用错误
	ErrCodeInternal       ErrorCode = "INTERNAL_ERROR"
	ErrCodeInvalidInput   ErrorCode = "INVALID_INPUT"
	ErrCodeNotFound       ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists  ErrorCode = "ALREADY_EXISTS"
	
	// 文件操作错误
	ErrCodeFileNotFound   ErrorCode = "FILE_NOT_FOUND"
	ErrCodeFileReadError  ErrorCode = "FILE_READ_ERROR"
	ErrCodeFileWriteError ErrorCode = "FILE_WRITE_ERROR"
	ErrCodeInvalidFormat  ErrorCode = "INVALID_FORMAT"
	
	// 解析错误
	ErrCodeParseError     ErrorCode = "PARSE_ERROR"
	ErrCodeRegexError     ErrorCode = "REGEX_ERROR"
	ErrCodeCellError      ErrorCode = "CELL_ERROR"
	
	// 验证错误
	ErrCodeValidation     ErrorCode = "VALIDATION_ERROR"
	ErrCodeConstraint     ErrorCode = "CONSTRAINT_ERROR"
	ErrCodeMissingField   ErrorCode = "MISSING_FIELD"
	
	// 业务逻辑错误
	ErrCodeHierarchy      ErrorCode = "HIERARCHY_ERROR"
	ErrCodeDuplicate      ErrorCode = "DUPLICATE_ERROR"
	ErrCodeInvalidLevel   ErrorCode = "INVALID_LEVEL"
)

// BaseError 基础错误结构
type BaseError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	StackTrace string    `json:"stack_trace,omitempty"`
}

// Error 实现error接口
func (e *BaseError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// GetCode 获取错误代码
func (e *BaseError) GetCode() ErrorCode {
	return e.Code
}

// GetMessage 获取错误消息
func (e *BaseError) GetMessage() string {
	return e.Message
}

// WithStackTrace 添加堆栈跟踪
func (e *BaseError) WithStackTrace() *BaseError {
	if e.StackTrace == "" {
		e.StackTrace = getStackTrace()
	}
	return e
}

// ParseError 解析错误
type ParseError struct {
	BaseError
	Row        int    `json:"row"`
	Column     int    `json:"column"`
	Content    string `json:"content"`
	Field      string `json:"field"`
	Expression string `json:"expression,omitempty"`
}

// NewParseError 创建解析错误
func NewParseError(row, column int, content, field, message string) *ParseError {
	return &ParseError{
		BaseError: BaseError{
			Code:      ErrCodeParseError,
			Message:   message,
			Timestamp: time.Now(),
		},
		Row:     row,
		Column:  column,
		Content: content,
		Field:   field,
	}
}

// Error 实现error接口
func (e *ParseError) Error() string {
	return fmt.Sprintf("[%s] 行%d列%d解析失败: %s (内容: '%s', 字段: %s)", 
		e.Code, e.Row, e.Column, e.Message, e.Content, e.Field)
}

// ValidationError 验证错误
type ValidationError struct {
	BaseError
	Field       string      `json:"field"`
	Value       interface{} `json:"value"`
	Constraint  string      `json:"constraint"`
	ExpectedValue interface{} `json:"expected_value,omitempty"`
}

// NewValidationError 创建验证错误
func NewValidationError(field string, value interface{}, constraint, message string) *ValidationError {
	return &ValidationError{
		BaseError: BaseError{
			Code:      ErrCodeValidation,
			Message:   message,
			Timestamp: time.Now(),
		},
		Field:      field,
		Value:      value,
		Constraint: constraint,
	}
}

// Error 实现error接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%s] 字段'%s'验证失败: %s (值: %v, 约束: %s)", 
		e.Code, e.Field, e.Message, e.Value, e.Constraint)
}

// SystemError 系统错误
type SystemError struct {
	BaseError
	Component string `json:"component"`
	Operation string `json:"operation"`
	Cause     error  `json:"cause,omitempty"`
}

// NewSystemError 创建系统错误
func NewSystemError(component, operation, message string, cause error) *SystemError {
	return &SystemError{
		BaseError: BaseError{
			Code:      ErrCodeInternal,
			Message:   message,
			Timestamp: time.Now(),
		},
		Component: component,
		Operation: operation,
		Cause:     cause,
	}
}

// Error 实现error接口
func (e *SystemError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s.%s失败: %s (原因: %v)", 
			e.Code, e.Component, e.Operation, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s.%s失败: %s", 
		e.Code, e.Component, e.Operation, e.Message)
}

// Unwrap 返回原始错误
func (e *SystemError) Unwrap() error {
	return e.Cause
}

// FileError 文件操作错误
type FileError struct {
	BaseError
	FilePath  string `json:"file_path"`
	Operation string `json:"operation"`
	Cause     error  `json:"cause,omitempty"`
}

// NewFileError 创建文件错误
func NewFileError(code ErrorCode, filepath, operation, message string, cause error) *FileError {
	return &FileError{
		BaseError: BaseError{
			Code:      code,
			Message:   message,
			Timestamp: time.Now(),
		},
		FilePath:  filepath,
		Operation: operation,
		Cause:     cause,
	}
}

// Error 实现error接口
func (e *FileError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] 文件操作失败 %s('%s'): %s (原因: %v)", 
			e.Code, e.Operation, e.FilePath, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] 文件操作失败 %s('%s'): %s", 
		e.Code, e.Operation, e.FilePath, e.Message)
}

// Unwrap 返回原始错误
func (e *FileError) Unwrap() error {
	return e.Cause
}

// HierarchyError 层级结构错误
type HierarchyError struct {
	BaseError
	Code1      string `json:"code1"`
	Code2      string `json:"code2,omitempty"`
	Level      int    `json:"level"`
	Operation  string `json:"operation"`
}

// NewHierarchyError 创建层级结构错误
func NewHierarchyError(code1, code2, operation, message string, level int) *HierarchyError {
	return &HierarchyError{
		BaseError: BaseError{
			Code:      ErrCodeHierarchy,
			Message:   message,
			Timestamp: time.Now(),
		},
		Code1:     code1,
		Code2:     code2,
		Level:     level,
		Operation: operation,
	}
}

// Error 实现error接口
func (e *HierarchyError) Error() string {
	if e.Code2 != "" {
		return fmt.Sprintf("[%s] 层级结构错误 %s('%s' -> '%s'): %s (层级: %d)", 
			e.Code, e.Operation, e.Code1, e.Code2, e.Message, e.Level)
	}
	return fmt.Sprintf("[%s] 层级结构错误 %s('%s'): %s (层级: %d)", 
		e.Code, e.Operation, e.Code1, e.Message, e.Level)
}

// ErrorList 错误列表
type ErrorList struct {
	Errors []error `json:"errors"`
}

// NewErrorList 创建错误列表
func NewErrorList() *ErrorList {
	return &ErrorList{
		Errors: make([]error, 0),
	}
}

// Add 添加错误
func (el *ErrorList) Add(err error) {
	if err != nil {
		el.Errors = append(el.Errors, err)
	}
}

// HasError 是否有错误
func (el *ErrorList) HasError() bool {
	return len(el.Errors) > 0
}

// Count 错误数量
func (el *ErrorList) Count() int {
	return len(el.Errors)
}

// Error 实现error接口
func (el *ErrorList) Error() string {
	if len(el.Errors) == 0 {
		return ""
	}
	
	if len(el.Errors) == 1 {
		return el.Errors[0].Error()
	}
	
	var messages []string
	for _, err := range el.Errors {
		messages = append(messages, err.Error())
	}
	
	return fmt.Sprintf("发生了%d个错误: [%s]", 
		len(el.Errors), strings.Join(messages, "; "))
}

// GetByType 根据错误类型过滤
func (el *ErrorList) GetByType(errorType ErrorCode) []error {
	var filtered []error
	for _, err := range el.Errors {
		if baseErr, ok := err.(*BaseError); ok && baseErr.Code == errorType {
			filtered = append(filtered, err)
		}
		if parseErr, ok := err.(*ParseError); ok && parseErr.Code == errorType {
			filtered = append(filtered, err)
		}
		if validErr, ok := err.(*ValidationError); ok && validErr.Code == errorType {
			filtered = append(filtered, err)
		}
		if sysErr, ok := err.(*SystemError); ok && sysErr.Code == errorType {
			filtered = append(filtered, err)
		}
		if fileErr, ok := err.(*FileError); ok && fileErr.Code == errorType {
			filtered = append(filtered, err)
		}
		if hierErr, ok := err.(*HierarchyError); ok && hierErr.Code == errorType {
			filtered = append(filtered, err)
		}
	}
	return filtered
}

// 辅助函数：获取堆栈跟踪
func getStackTrace() string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	
	var traces []string
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		traces = append(traces, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}
	
	return strings.Join(traces, "\n")
}

// IsErrorType 检查错误是否为指定类型
func IsErrorType(err error, code ErrorCode) bool {
	if err == nil {
		return false
	}
	
	switch e := err.(type) {
	case *BaseError:
		return e.Code == code
	case *ParseError:
		return e.Code == code
	case *ValidationError:
		return e.Code == code
	case *SystemError:
		return e.Code == code
	case *FileError:
		return e.Code == code
	case *HierarchyError:
		return e.Code == code
	}
	
	return false
}

// 简化的错误创建函数，兼容旧代码

// NewInternalError 创建内部错误
func NewInternalError(message string, cause error) error {
	return NewSystemError("internal", "process", message, cause)
}

// NewParsingError 创建解析错误
func NewParsingError(message string, cause error) error {
	return NewSystemError("parser", "parse", message, cause)
}

// NewNotFoundError 创建未找到错误
func NewNotFoundError(message string) error {
	return &BaseError{
		Code:      ErrCodeNotFound,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// SimpleValidationError 创建简单验证错误
func SimpleValidationError(message string) error {
	return &BaseError{
		Code:      ErrCodeValidation,
		Message:   message,
		Timestamp: time.Now(),
	}
}