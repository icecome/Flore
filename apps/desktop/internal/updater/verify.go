package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
)

// updatePublicKeyRaw 是更新签名校验公钥（Ed25519，原始 32 字节，base64 编码）。
// 对应私钥由发布方保管（首次生成时输出到用户目录，勿提交仓库），
// 发布流水线使用 scripts/sign-update.mjs 对每个资产文件的 SHA256 摘要签名，
// 将签名写入 update.json 的 asset.signature 字段。
// 轮换密钥时替换本常量并在发布流程中同步更新。
const updatePublicKeyRaw = "IL0y16Rf197F/Eyrg7wMvzKMbXQ8R4bCNc/p3t0ltH0="

var updatePublicKey = mustDecodeUpdatePublicKey()

func mustDecodeUpdatePublicKey() ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(updatePublicKeyRaw)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		// 常量在编译期即固定，解码失败属编程错误，直接 panic 以在测试/启动时暴露
		panic("updater: invalid embedded update public key")
	}
	return ed25519.PublicKey(raw)
}

// verifyAssetSignature 校验资产文件摘要的 Ed25519 签名（使用内嵌公钥）。
// digest 为资产文件原始字节的 SHA256 摘要（32 字节），与 scripts/sign-update.mjs
// 的签名输入保持一致（签名的 message 即该摘要，而非文件本身）。
func verifyAssetSignature(asset *Asset, digest []byte) error {
	return verifyAssetSignatureWithKey(updatePublicKey, asset, digest)
}

// verifyAssetSignatureWithKey 使用指定公钥校验签名，供 verifyAssetSignature 与单元测试共用。
func verifyAssetSignatureWithKey(pub ed25519.PublicKey, asset *Asset, digest []byte) error {
	if asset == nil {
		return fmt.Errorf("签名校验失败: asset 为空")
	}
	if asset.Signature == "" {
		return fmt.Errorf("签名校验失败: 资产 %s 缺少 signature 字段", asset.FileName)
	}
	sig, err := base64.StdEncoding.DecodeString(asset.Signature)
	if err != nil {
		return fmt.Errorf("签名校验失败: signature 不是合法 base64: %w", err)
	}
	if !ed25519.Verify(pub, digest, sig) {
		return fmt.Errorf("签名校验失败: 资产 %s 签名无效（可能被篡改或由非授权方发布）", asset.FileName)
	}
	return nil
}
