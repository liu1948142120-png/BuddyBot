// 内置手册：随应用打包，离线可看。内容与分发文档同源（v1.0.21 起）。
import integrationMd from "./integration.md?raw";
import featuresMd from "./features.md?raw";
import macosMd from "./macos.md?raw";
import updatesMd from "./updates.md?raw";

export interface Manual {
  id: string;
  label: string;
  md: string;
}

export const MANUALS: Manual[] = [
  {
    id: "integration",
    label: "接入手册",
    md: integrationMd,
  },
  {
    id: "features",
    label: "功能介绍",
    md: featuresMd,
  },
  {
    id: "macos",
    label: "macOS 版说明",
    md: macosMd,
  },
  {
    id: "updates",
    label: "更新日志",
    md: updatesMd,
  },
];
