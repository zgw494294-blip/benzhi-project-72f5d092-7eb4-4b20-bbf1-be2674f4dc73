package persistence

// Fingerprint 描述一次幂等提交的请求指纹，包含请求方法、目标资源、操作者
// 与规范化请求内容摘要。仓储层在幂等重放时用它判定两次提交是否属于同一
// 请求：只有全部维度一致时才允许把旧结果当成本次请求的幂等重放响应返回。
type Fingerprint struct {
	present     bool
	method      string
	resource    string
	actorID     string
	actorRole   string
	requestHash string
}

// NewFingerprint 构造一个携带方法、目标资源、操作者与规范化请求内容摘要的指纹。
func NewFingerprint(method, resource, actorID, actorRole, requestHash string) Fingerprint {
	return Fingerprint{present: true, method: method, resource: resource, actorID: actorID, actorRole: actorRole, requestHash: requestHash}
}

// firstFingerprint 从可变参数中取出首个指纹；调用方未提供时返回空指纹，
// 以保持仓储层直接调用方（如测试）在不传递指纹时的向后兼容。
func firstFingerprint(fingerprints []Fingerprint) Fingerprint {
	if len(fingerprints) == 0 {
		return Fingerprint{}
	}
	return fingerprints[0]
}

// matches 判断存储的幂等记录与当前请求指纹是否一致。任一维度（方法、目标
// 资源、操作者或规范化请求内容摘要）不同都视为不同请求。
func (f Fingerprint) matches(record IdempotentRecord) bool {
	if !f.present {
		return true
	}
	if record.Method == "" && record.Resource == "" && record.RequestHash == "" {
		// 旧记录未存储指纹：仅有方法+资源+操作者+内容指纹时拒绝重放。
		return false
	}
	return f.method == record.Method && f.resource == record.Resource && f.actorID == record.ActorID && f.actorRole == record.ActorRole && f.requestHash == record.RequestHash
}
