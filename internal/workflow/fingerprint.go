package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"acoustic-annotation-release/internal/persistence"
)

// methodForOperation 返回幂等指纹中的请求方法。操作名已隐含路由与方法，
// 这里显式记录方法以满足“请求方法一致”的幂等重放约束：同一 Idempotency-Key
// 不得跨方法（如 POST 与 PATCH）复用。
func methodForOperation(operation string) string {
	if operation == "clip.privacy_updated" {
		return http.MethodPatch
	}
	return http.MethodPost
}

// resourcePath 返回幂等指纹中的目标资源描述，结合批次与子资源标识，
// 使得同一 Idempotency-Key 指向不同子资源（如不同 clipID 或 conflictID）时
// 视为不同请求。
func resourcePath(batchID string, subResources ...string) string {
	parts := []string{batchID}
	for _, item := range subResources {
		item = strings.TrimSpace(item)
		if item != "" {
			parts = append(parts, item)
		}
	}
	return strings.Join(parts, "/")
}

// contentHash 计算规范化请求内容的稳定摘要。结构体字段顺序在 Go 中固定，
// 序列化结果对相同输入稳定，足以判定请求内容是否变化。无请求体时返回空串。
func contentHash(content any) string {
	if content == nil {
		return ""
	}
	data, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	if len(data) == 0 || string(data) == "null" {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// idempotencyFingerprint 构造一个携带方法、目标资源、操作者与规范化请求内容
// 摘要的仓储层指纹，供幂等重放时判定两次提交是否属于同一请求。
func idempotencyFingerprint(method, resource, actorID, actorRole, requestHash string) persistence.Fingerprint {
	return persistence.NewFingerprint(method, resource, actorID, actorRole, requestHash)
}
