import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ModalForm,
  PageContainer,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { App, Button, Input, Modal, Select, Space, Tag, Typography } from 'antd';
import React, { useRef, useState } from 'react';
import {
  CS_TICKET_STATUS_LABELS,
  CS_TICKET_TYPE_LABELS,
} from '@/services/youwumeng/labels';
import { formatDateTime } from '@/services/youwumeng/format';
import {
  createCsTicket,
  listCsTickets,
  updateCsTicket,
} from '@/services/youwumeng/tickets';
import type { CsTicket, CsTicketStatus } from '@/services/youwumeng/typings';

const DAY_MS = 24 * 3600 * 1000;

/** 客服工单（设计文档 9.3）：登记 + 流转，超 24h 未处理标红 */
const CsTickets: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const actionRef = useRef<ActionType>(null);
  const [flowTarget, setFlowTarget] = useState<CsTicket | null>(null);
  const [flowStatus, setFlowStatus] = useState<CsTicketStatus>('processing');
  const [flowRemark, setFlowRemark] = useState('');

  const columns: ProColumns<CsTicket>[] = [
    {
      title: '类型',
      dataIndex: 'type',
      width: 130,
      valueEnum: Object.fromEntries(
        Object.entries(CS_TICKET_TYPE_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, r) => <Tag>{CS_TICKET_TYPE_LABELS[r.type]}</Tag>,
    },
    { title: '用户姓名', dataIndex: 'user_name', width: 110 },
    { title: '联系电话', dataIndex: 'phone', width: 130 },
    {
      title: '诉求内容',
      dataIndex: 'content',
      ellipsis: true,
      render: (_, r) => (
        <Typography.Paragraph ellipsis={{ rows: 2 }} style={{ marginBottom: 0 }}>
          {r.content}
        </Typography.Paragraph>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      valueEnum: Object.fromEntries(
        Object.entries(CS_TICKET_STATUS_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, r) => {
        const overdue =
          r.status === 'pending' && Date.now() - new Date(r.created_at).getTime() > DAY_MS;
        return (
          <Space size={4}>
            <Tag color={r.status === 'done' ? 'success' : r.status === 'processing' ? 'processing' : 'warning'}>
              {CS_TICKET_STATUS_LABELS[r.status]}
            </Tag>
            {overdue && <Tag color="error">超时未处理</Tag>}
          </Space>
        );
      },
    },
    { title: '登记人', dataIndex: 'created_by', width: 100 },
    {
      title: '关联订单',
      dataIndex: 'related_order_no',
      width: 180,
      render: (_, r) => r.related_order_no ?? '-',
    },
    { title: '登记时间', dataIndex: 'created_at', width: 150, render: (_, r) => formatDateTime(r.created_at) },
    { title: '跟进备注', dataIndex: 'remark', ellipsis: true, render: (_, r) => r.remark ?? '-' },
    {
      title: '操作',
      valueType: 'option',
      width: 90,
      render: (_, r) =>
        access.canAdmin ? (
          <Typography.Link
            onClick={() => {
              setFlowTarget(r);
              setFlowStatus(r.status === 'pending' ? 'processing' : 'done');
              setFlowRemark('');
            }}
          >
            流转
          </Typography.Link>
        ) : null,
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<CsTicket>
        headerTitle="客服工单"
        rowKey="ticket_id"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 'auto' }}
        request={async (params) => {
          const data = await listCsTickets({
            status: params.status as CsTicketStatus | undefined,
            type: params.type as CsTicket['type'] | undefined,
            keyword: params.content ?? params.user_name,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        toolBarRender={() => [
          <ModalForm
            key="create"
            title="登记工单"
            trigger={<Button type="primary">登记工单</Button>}
            onFinish={async (values) => {
              await createCsTicket({
                type: values.type as CsTicket['type'],
                user_name: values.user_name,
                phone: values.phone,
                content: values.content,
                related_order_no: values.related_order_no || undefined,
              });
              message.success('已登记');
              actionRef.current?.reload();
              return true;
            }}
          >
            <ProFormSelect
              name="type"
              label="类型"
              rules={[{ required: true }]}
              options={Object.entries(CS_TICKET_TYPE_LABELS).map(([value, label]) => ({ value, label }))}
            />
            <ProFormText name="user_name" label="用户姓名" />
            <ProFormText name="phone" label="联系电话" rules={[{ required: true, message: '请填写联系方式' }]} />
            <ProFormTextArea name="content" label="诉求内容" rules={[{ required: true, message: '请填写诉求内容' }]} />
            <ProFormText name="related_order_no" label="关联订单号（选填）" />
          </ModalForm>,
        ]}
      />
      <Modal
        title={flowTarget ? `流转工单：${flowTarget.user_name}` : ''}
        open={!!flowTarget}
        onCancel={() => setFlowTarget(null)}
        onOk={async () => {
          if (!flowTarget) return;
          await updateCsTicket(flowTarget.ticket_id, { status: flowStatus, remark: flowRemark || undefined });
          message.success('已流转');
          setFlowTarget(null);
          actionRef.current?.reload();
        }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Select
            style={{ width: '100%' }}
            value={flowStatus}
            onChange={(v) => setFlowStatus(v as CsTicketStatus)}
            options={Object.entries(CS_TICKET_STATUS_LABELS).map(([value, label]) => ({ value, label }))}
          />
          <Input.TextArea
            rows={3}
            placeholder="跟进备注（如：已转管理员退款）"
            value={flowRemark}
            onChange={(e) => setFlowRemark(e.target.value)}
          />
        </Space>
      </Modal>
    </PageContainer>
  );
};

export default CsTickets;
