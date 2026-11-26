import { App as AntApp, Button, Dropdown, Space } from "antd";
import type { MenuProps } from "antd";
import { DownOutlined } from "@ant-design/icons";

export interface RowAction {
  key: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  loading?: boolean;
  onClick: () => void;
  confirm?: { title: string; description?: string };
}

/**
 * Compact table row actions: a couple of text links plus an overflow menu.
 * Destructive entries use a confirm dialog instead of inline Popconfirm so
 * they can live inside the dropdown.
 */
export default function RowActions({ actions, inline = 2 }: { actions: RowAction[]; inline?: number }) {
  const { modal } = AntApp.useApp();

  const run = (action: RowAction) => {
    if (action.confirm) {
      modal.confirm({
        title: action.confirm.title,
        content: action.confirm.description,
        okText: "确认",
        cancelText: "取消",
        okButtonProps: { danger: action.danger },
        onOk: action.onClick,
      });
      return;
    }
    action.onClick();
  };

  const shown = actions.slice(0, inline);
  const rest = actions.slice(inline);
  const menu: MenuProps = {
    items: rest.map((a) => ({
      key: a.key,
      label: a.label,
      danger: a.danger,
      disabled: a.disabled,
    })),
    onClick: ({ key }) => {
      const action = rest.find((a) => a.key === key);
      if (action && !action.disabled) run(action);
    },
  };

  if (actions.length === 0) return null;
  return (
    <Space size={12} wrap={false}>
      {shown.map((a) => (
        <Button
          key={a.key}
          type="link"
          size="small"
          danger={a.danger}
          disabled={a.disabled}
          loading={a.loading}
          style={{ padding: 0, height: "auto" }}
          onClick={() => run(a)}
        >
          {a.label}
        </Button>
      ))}
      {rest.length > 0 && (
        <Dropdown menu={menu} trigger={["click"]}>
          <Button type="link" size="small" style={{ padding: 0, height: "auto" }}>
            更多 <DownOutlined style={{ fontSize: 10 }} />
          </Button>
        </Dropdown>
      )}
    </Space>
  );
}
