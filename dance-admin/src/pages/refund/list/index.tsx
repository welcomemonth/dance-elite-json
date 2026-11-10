import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history, useLocation } from '@umijs/max';
import { Tag, Typography } from 'antd';
import React, { useRef } from 'react';
import { formatDateTime } from '@/services/youwumeng/format';
import { REFUND_TICKET_STATUS_LABELS } from '@/services/youwumeng/labels';
import { listRefundTickets } from '@/services/youwumeng/refunds';
import { listEvents } from '@/services/youwumeng/events';
import type {
  RefundTicketBrief,
  RefundTicketStatus,
} from '@/services/youwumeng/typings';

const STATUS_COLORS: Record<RefundTicketStatus, string> = {
  pending: 'warning',
  processing: 'processing',
  approved: 'success',
  rejected: 'default',
  refund_failed: 'error',
};

const useInitialKeyword = (): string => {
  const { search } = useLocation();
  return new URLSearchParams(search).get('keyword') ?? '';
};

const RefundList: React.FC = () => {
  const actionRef = useRef<ActionType>(null);
  const initialKeyword = useInitialKeyword();

  const columns: ProColumns<RefundTicketBrief>[] = [
    {
      title: '工单号',
      dataIndex: 'ticket_no',
      width: 160,
      render: (_, record) => (
        <Typography.Link onClick={() => history.push(`/order/refunds/${record.ticket_id}`)}>
          {record.ticket_no}
        </Typography.Link>
      ),
    },
    {
      title: '关联订单号',
      dataIndex: 'order_no',
      width: 200,
      render: (_, record) => (
        <Typography.Link onClick={() => history.push(`/order/detail/${record.order_no}`)}>
          {record.order_no}
        </Typography.Link>
      ),
    },
    { title: '赛事', dataIndex: 'event_name', width: 240, search: false },
    {
      title: '赛事',
      dataIndex: 'event_id',
      hideInTable: true,
      valueType: 'select',
      request: async () => {
        const data = await listEvents({ page: 1, page_size: 100 });
        return data.list.map((e) => ({ label: e.name, value: e.event_id }));
      },
    },
    { title: '选手姓名', dataIndex: 'player_name', width: 100, search: false },
    {
      title: '关键字',
      dataIndex: 'keyword',
      hideInTable: true,
      fieldProps: { placeholder: '工单号 / 订单号 / 姓名' },
    },
    { title: '登记来源', dataIndex: 'applicant', width: 110, search: false },
    { title: '诉求摘要', dataIndex: 'reason', width: 180 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      valueEnum: Object.fromEntries(
        Object.entries(REFUND_TICKET_STATUS_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, record) => (
        <Tag color={STATUS_COLORS[record.status]}>
          {REFUND_TICKET_STATUS_LABELS[record.status]}
        </Tag>
      ),
    },
    {
      title: '登记时间',
      dataIndex: 'created_at',
      width: 150,
      valueType: 'dateTimeRange',
      search: { transform: (value) => ({ created_from: value?.[0], created_to: value?.[1] }) },
      render: (_, record) => formatDateTime(record.created_at),
    },
    { title: '处理人', dataIndex: 'handler_name', width: 100, search: false, render: (_, r) => r.handler_name ?? '-' },
    {
      title: '处理时间',
      dataIndex: 'handled_at',
      width: 150,
      search: false,
      render: (_, record) => formatDateTime(record.handled_at),
    },
    {
      title: '操作',
      valueType: 'option',
      width: 80,
      render: (_, record) =>
        ['pending', 'refund_failed'].includes(record.status) ? (
          <Typography.Link onClick={() => history.push(`/order/refunds/${record.ticket_id}`)}>
            处理
          </Typography.Link>
        ) : (
          <Typography.Link onClick={() => history.push(`/order/refunds/${record.ticket_id}`)}>
            查看
          </Typography.Link>
        ),
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<RefundTicketBrief>
        headerTitle="退款工单"
        rowKey="ticket_id"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 'auto', defaultCollapsed: false }}
        form={{ initialValues: { keyword: initialKeyword || undefined } }}
        request={async (params) => {
          const range = params.created_at as [string, string] | undefined;
          const data = await listRefundTickets({
            status: params.status as RefundTicketStatus | undefined,
            event_id: params.event_id as number | undefined,
            created_from: range?.[0],
            created_to: range?.[1],
            keyword: (params.keyword as string) || initialKeyword || undefined,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        options={false}
      />
    </PageContainer>
  );
};

export default RefundList;
