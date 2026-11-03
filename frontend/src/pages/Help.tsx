import { useMemo, useState } from "react";
import { marked } from "marked";
import DOMPurify from "dompurify";
import { MANUALS } from "../content/manuals";
import { useT } from "../i18n";

/** 渲染单份手册为净化后的 HTML（与 Chat 的 markdown 渲染同栈：marked + DOMPurify） */
function render(md: string): string {
  const html = marked.parse(md, { async: false, gfm: true, breaks: false });
  return DOMPurify.sanitize(html);
}

export default function Help() {
  const t = useT();
  const [tab, setTab] = useState(MANUALS[0].id);
  const current = MANUALS.find((m) => m.id === tab) ?? MANUALS[0];
  const html = useMemo(() => render(current.md), [current.md]);

  return (
    <div className="page help-page">
      <div className="page-head">
        <div>
          <h1>{t("使用手册")}</h1>
          <p>{t("内置离线文档 · 接入、排障、功能说明随版本更新")}</p>
        </div>
        <div className="help-tabs" role="tablist">
          {MANUALS.map((m) => (
            <button
              key={m.id}
              role="tab"
              aria-selected={m.id === tab}
              className={`help-tab${m.id === tab ? " on" : ""}`}
              onClick={() => setTab(m.id)}
            >
              {t(m.label)}
            </button>
          ))}
        </div>
      </div>
      <div className="help-doc card" dangerouslySetInnerHTML={{ __html: html }} />
    </div>
  );
}
