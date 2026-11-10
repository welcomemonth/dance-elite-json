import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { Alert, Tag } from 'antd';
import React, { useRef } from 'react';
import { formatDateTime } from '@/services/youwumeng/format';
import { listLogs } from '@/services/youwumeng/system';
import type { LogEntry } from '@/services/youwumeng/typings';

const ACTION_LABELS: Record<string, string> = {
  login: '登录',
  create_event: '创建赛事',
  update_event: '修改赛事',
  publish_event: '发布赛事',
  offline_event: '下线赛事',
  set_group_quota: '调整名额',
  close_group: '关闭组别',
  reopen_group: '重开组别',
  manual_refund: '人工退款',
  reveal_phone: '查看手机号',
  correct_score: '成绩纠错',
  adjust_leaderboard: '榜单调整',
  update_content: '修改内容',
  create_account: '创建账号',
  update_account: '修改账号',
  export_data: '导出数据',
  approve_refund: '同意退款',
  reject_refund: '驳回退款',
  audio_review_token: '生成审核链接',
};

const ACTION_COLOR: Record<string, string> = {
  manual_refund: 'volcano',
  reveal_phone: 'orange',
  correct_score: 'red',
  login: 'blue',
};

/** 操作日志（设计文档 10.2）：全量查询，不可删除 */
const Logs: React.FC = () => {
  const access = useAccess();
  const actionRef = useRef<ActionType>(null);

  const columns: ProColumns<LogEntry>[] = [
    { title: '时间', dataIndex: 'created_at', width: 170, render: (_, r) => formatDateTime(r.created_at) },
    {
      title: '操作人',
      dataIndex: 'admin_name',
      width: 120,
      render: (_, r) => r.admin_name,
    },
    {
      title: '动作',
      dataIndex: 'action',
      width: 150,
      valueEnum: Object.fromEntries(
        Object.entries(ACTION_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, r) => (
        <Tag color={ACTION_COLOR[r.action] ?? 'default'}>{ACTION_LABELS[r.action] ?? r.action}</Tag>
      ),
    },
    { title: '对象', dataIndex: 'target_type', width: 200, render: (_, r) => `${r.target_type}#${r.target_id}` },
    { title: '详情', dataIndex: 'detail', ellipsis: true, render: (_, r) => JSON.stringify(r.detail) },
  ];

  if (!access.isSuperAdmin) {
    return (
      <PageContainer title={false}>
        <Alert type="warning" showIcon message="操作日志仅超级管理员可查看。" />
      </PageContainer>
    );
  }

  return (
    <PageContainer title={false}>
      <ProTable<LogEntry>
        headerTitle="操作日志"
        rowKey="log_id"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 'auto' }}
        request={async (params) => {
          const data = await listLogs({
            action: params.action as string | undefined,
            admin_id: params.admin_name ? Number(params.admin_name) : undefined,
            created_from: params.created_at ? String(params.created_at[0]) : undefined,
            created_to: params.created_at ? String(params.created_at[1]) : undefined,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
      />
    </PageContainer>
  );
};

export default Logs;
