import { useState, useRef, useEffect } from "react";
import { MessageCircle, X, Copy, Check, ExternalLink } from "lucide-react";
import { accountsApi } from "../../services/api";
import { toast } from "../common/Feedback";
import { useT } from "../../i18n";

// ===== 在此填写你的联系方式 =====
const CONTACT_INFO = {
  wechat: "jyy6783",      // 微信号
  qq: "1948142120",       // QQ号
  qqGroup: "145051716",   // QQ群号
  homepage: "https://sq.jyy01.cn/h5", // 主页地址
};

export default function HelpFAB() {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const t = useT();
  const cardRef = useRef<HTMLDivElement>(null);

  // 点击外部关闭
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (cardRef.current && !cardRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const copy = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(label);
      toast.success(t("已复制"), text);
      setTimeout(() => setCopied(null), 2000);
    } catch {
      toast.error(t("复制失败"), t("请手动复制"));
    }
  };

  const openHome = () => {
    void accountsApi.openURL(CONTACT_INFO.homepage).catch(() => {});
  };

  return (
    <div className="help-fab-wrap" ref={cardRef}>
      {/* 联系卡片 */}
      {open && (
        <div className="help-card">
          <div className="help-card-head">
            <b>{t("联系作者")}</b>
            <button className="icon-btn" onClick={() => setOpen(false)} aria-label={t("关闭")}>
              <X size={14} strokeWidth={2.4} />
            </button>
          </div>
          <div className="help-card-body">
            <div className="help-row">
              <span className="help-label">{t("微信")}</span>
              <code className="help-value">{CONTACT_INFO.wechat}</code>
              <button
                className="help-copy"
                onClick={() => copy(CONTACT_INFO.wechat, "wechat")}
                aria-label={t("复制微信号")}
              >
                {copied === "wechat" ? (
                  <Check size={13} strokeWidth={2.4} />
                ) : (
                  <Copy size={13} strokeWidth={2.4} />
                )}
              </button>
            </div>
            <div className="help-row">
              <span className="help-label">{t("QQ")}</span>
              <code className="help-value">{CONTACT_INFO.qq}</code>
              <button
                className="help-copy"
                onClick={() => copy(CONTACT_INFO.qq, "qq")}
                aria-label={t("复制QQ号")}
              >
                {copied === "qq" ? (
                  <Check size={13} strokeWidth={2.4} />
                ) : (
                  <Copy size={13} strokeWidth={2.4} />
                )}
              </button>
            </div>
            <div className="help-row">
              <span className="help-label">{t("QQ群")}</span>
              <code className="help-value">{CONTACT_INFO.qqGroup}</code>
              <button
                className="help-copy"
                onClick={() => copy(CONTACT_INFO.qqGroup, "qqGroup")}
                aria-label={t("复制QQ群号")}
              >
                {copied === "qqGroup" ? (
                  <Check size={13} strokeWidth={2.4} />
                ) : (
                  <Copy size={13} strokeWidth={2.4} />
                )}
              </button>
            </div>
          </div>
          <div className="help-card-foot">
            <button className="help-home-btn" onClick={openHome}>
              <ExternalLink size={12} strokeWidth={2.4} />
              {t("访问主页")}
            </button>
          </div>
        </div>
      )}

      {/* 悬浮按钮 */}
      <button
        className={`help-fab${open ? " on" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-label={t("联系作者")}
        data-tip={open ? t("关闭") : t("联系作者")}
      >
        {open ? (
          <X size={20} strokeWidth={2.4} />
        ) : (
          <MessageCircle size={20} strokeWidth={2} />
        )}
      </button>
    </div>
  );
}
