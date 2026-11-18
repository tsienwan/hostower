// en 词典：容器应用按镜像仓库检测更新。
const dict: Record<string, string> = {
  "拉取新构建并重建容器": "Pull the new build and recreate the container",
  "同一个镜像标签在仓库里有了新的构建（通常是小版本或安全修复），更新会重新拉取并重建容器。": "The same image tag now points to a new build in the registry (usually a minor release or security fix); updating pulls it and recreates the container.",
  "镜像有新构建": "New image build",
  "镜像仓库里已经有这个大版本，但目录还没收录：要先确认能不能原地升级，Hostower 不会自动换过去。": "This major version already exists in the registry but is not in the catalog yet: in-place upgradability must be checked first, so Hostower will not switch to it automatically.",
  "上游有新大版本 {v}": "Upstream major {v} available",
};

export default dict;
