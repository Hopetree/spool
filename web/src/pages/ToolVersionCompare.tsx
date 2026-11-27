import { useMemo, useState, type ReactNode } from "react";
import { Button, Card, Empty, Segmented, Select, Space, Typography } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { getTool, listVersions, type ToolParam, type ToolVersion } from "../api/tools";
import PageHeader from "../components/PageHeader";
import VersionDiff, { diffStats, mergeDiffStats, type DiffMode, type DiffStats } from "../components/VersionDiff";

function formatParam(param: ToolParam): string {
  return [
    `${param.name}（${param.label || param.name}）`,
    `类型=${param.type}`,
    `必填=${param.required ? "是" : "否"}`,
    `默认=${param.default || "—"}`,
    `候选项=${param.options.length ? param.options.join(", ") : "—"}`,
    `敏感=${param.sensitive ? "是" : "否"}`,
    `说明=${param.description || "—"}`,
  ].join(" ");
}

function basicInfoText(version: ToolVersion): string {
  const lines = [
    `版本号: v${version.version_no}`,
    `类型: ${version.type === "python" ? "Python" : "Shell"}`,
    `解释器: ${version.interpreter || "—"}`,
    `超时: ${version.timeout_sec} 秒`,
    `sudo -n: ${version.sudo ? "是" : "否"}`,
    `准备命令: ${version.prepare_command || "（无）"}`,
    `变更说明: ${version.change_note || "—"}`,
    `创建人: ${version.created_by_name || version.created_by || "—"}`,
  ];
  if (version.params.length === 0) {
    lines.push("参数: 无");
  } else {
    lines.push("参数:");
    version.params.forEach((param, index) => {
      lines.push(`  ${index + 1}. ${formatParam(param)}`);
    });
  }
  return lines.join("\n");
}

function DiffSection({
  title,
  stats,
  open,
  onToggle,
  children,
}: {
  title: string;
  stats: DiffStats;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div style={{ border: "1px solid #f0f0f0", borderRadius: 8, marginBottom: 16, overflow: "hidden" }}>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 12,
          padding: "10px 12px",
          background: "#fafafa",
        }}
      >
        <Space size={10}>
          <Typography.Text strong>{title}</Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {stats.added || stats.deleted ? `新增 ${stats.added} 行 / 删除 ${stats.deleted} 行` : "无变化"}
          </Typography.Text>
        </Space>
        <Button type="link" size="small" onClick={onToggle}>
          {open ? "收起" : "展开"}
        </Button>
      </div>
      {open && children}
    </div>
  );
}

export default function ToolVersionCompare() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [mode, setMode] = useState<DiffMode>("split");
  const [openBasic, setOpenBasic] = useState(true);
  const [openScript, setOpenScript] = useState(true);

  const { data: detail, isLoading: detailLoading } = useQuery({
    queryKey: ["tool", id],
    queryFn: () => getTool(id!),
    enabled: !!id,
  });
  const { data: versionData = [], isLoading: versionsLoading } = useQuery({
    queryKey: ["tool-versions", id],
    queryFn: () => listVersions(id!),
    enabled: !!id,
  });

  const versions = useMemo(
    () => [...versionData].sort((a, b) => b.version_no - a.version_no),
    [versionData],
  );
  const versionNos = useMemo(() => new Set(versions.map((v) => v.version_no)), [versions]);

  const requestedFrom = Number(searchParams.get("from"));
  const requestedTo = Number(searchParams.get("to"));
  const fallbackTo = versions[0]?.version_no;
  const fallbackFrom = versions[1]?.version_no ?? fallbackTo;
  const fromNo = versionNos.has(requestedFrom) ? requestedFrom : fallbackFrom;
  const toNo = versionNos.has(requestedTo) ? requestedTo : fallbackTo;
  const fromVersion = versions.find((v) => v.version_no === fromNo);
  const toVersion = versions.find((v) => v.version_no === toNo);

  const basicStats = useMemo(
    () =>
      fromVersion && toVersion
        ? diffStats(basicInfoText(fromVersion), basicInfoText(toVersion))
        : { added: 0, deleted: 0 },
    [fromVersion, toVersion],
  );
  const scriptStats = useMemo(
    () =>
      fromVersion && toVersion
        ? diffStats(fromVersion.content, toVersion.content)
        : { added: 0, deleted: 0 },
    [fromVersion, toVersion],
  );
  const totalStats = mergeDiffStats(basicStats, scriptStats);

  const setVersion = (key: "from" | "to", value: number) => {
    const next = new URLSearchParams(searchParams);
    next.set("from", String(key === "from" ? value : fromNo));
    next.set("to", String(key === "to" ? value : toNo));
    setSearchParams(next, { replace: true });
  };

  const versionsOpen = openBasic && openScript;
  const toggleAll = () => {
    const next = !versionsOpen;
    setOpenBasic(next);
    setOpenScript(next);
  };

  const versionOptions = versions.map((v) => ({ value: v.version_no, label: `v${v.version_no}` }));
  const loading = detailLoading || versionsLoading;

  return (
    <div>
      <PageHeader
        back={{ label: "返回工具", onClick: () => navigate(`/tools/${id}`) }}
        title="版本比较"
        subtitle={detail?.name}
        crumbs={[
          { title: "工具", to: "/tools" },
          { title: detail?.name || "工具详情", to: `/tools/${id}` },
          { title: "版本比较" },
        ]}
      />

      {loading ? (
        <Card loading />
      ) : versions.length === 0 ? (
        <Card>
          <Empty description="暂无可比较的版本" />
        </Card>
      ) : (
        <>
          <Card size="small" style={{ marginBottom: 16 }} styles={{ body: { padding: 12 } }}>
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                gap: 16,
                flexWrap: "wrap",
              }}
            >
              <Space wrap size={10}>
                <span>基准版本</span>
                <Select
                  size="small"
                  style={{ width: 110 }}
                  value={fromNo}
                  onChange={(value) => setVersion("from", value)}
                  options={versionOptions}
                />
                <span>对比版本</span>
                <Select
                  size="small"
                  style={{ width: 110 }}
                  value={toNo}
                  onChange={(value) => setVersion("to", value)}
                  options={versionOptions}
                />
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {totalStats.added || totalStats.deleted
                    ? `其中 ${totalStats.added} 行新增，${totalStats.deleted} 行删除`
                    : "两个版本内容一致"}
                </Typography.Text>
                <Button type="link" size="small" onClick={toggleAll} style={{ paddingInline: 0 }}>
                  {versionsOpen ? "收起全部" : "展开全部"}
                </Button>
              </Space>
              <Segmented
                value={mode}
                onChange={(value) => setMode(value as DiffMode)}
                options={[
                  { value: "unified", label: "统一" },
                  { value: "split", label: "拆分" },
                ]}
              />
            </div>
          </Card>

          {fromVersion && toVersion ? (
            <>
              <DiffSection
                title="基本信息"
                stats={basicStats}
                open={openBasic}
                onToggle={() => setOpenBasic((value) => !value)}
              >
                <VersionDiff
                  oldText={basicInfoText(fromVersion)}
                  newText={basicInfoText(toVersion)}
                  mode={mode}
                />
              </DiffSection>
              <DiffSection
                title="脚本内容"
                stats={scriptStats}
                open={openScript}
                onToggle={() => setOpenScript((value) => !value)}
              >
                <VersionDiff oldText={fromVersion.content} newText={toVersion.content} mode={mode} />
              </DiffSection>
            </>
          ) : (
            <Card>
              <Empty description="请选择两个版本进行比较" />
            </Card>
          )}
        </>
      )}
    </div>
  );
}
