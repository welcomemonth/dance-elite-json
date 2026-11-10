import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history, useSearchParams } from '@umijs/max';
import { App, Button, Space, Switch, Tag, Typography } from 'antd';
import React, { useRef, useState } from 'react';
import { formatDateTime, formatMoney } from '@/services/youwumeng/format';
import {
  AGE_LEVEL_OPTIONS,
  DANCE_TYPE_LABELS,
  ORDER_SOURCE_LABELS,
  ORDER_STATUS_LABELS,
} from '@/services/youwumeng/labels';
import { exportOrders, listOrders } from '@/services/youwumeng/orders';
import { listEvents } from '@/services/youwumeng/events';
import type {
  AgeLevel,
  DanceType,
  OrderBrief,
  OrderSource,
  OrderStatus,
} from '@/services/youwumeng/typings';

const STATUS_COLORS: Record<OrderStatus, string> = {
  paid: 'success',
  canceled: 'default',
  refunding: 'processing',
  refunded: 'warning',
};

const OrderList: React.FC = () => {
  const { message } = App.useApp();
  const actionRef = useRef<ActionType>(null);
  const [searchParams] = useSearchParams();
  const urlEventId = searchParams.get('event_id')
    ? Number(searchParams.get('event_id'))
    : undefined;
  const [includeCanceled, setIncludeCanceled] = useState(false);

  const handleExport = async () => {
    try {
      await exportOrders({ include_canceled: includeCanceled });
      message.success('导出成功');
    } catch (e) {
      message.error(e instanceof Error ? e.message : '导出失败');
    }
  };

  const columns: ProColumns<OrderBrief>[] = [
    {
      title: '订单号',
      dataIndex: 'order_no',
      width: 200,
      render: (_, record) => (
        <Typography.Link
          onClick={() => history.push(`/order/detail/${record.order_no}`)}
        >
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
    {
      title: '组别',
      dataIndex: 'level',
      width: 70,
      valueEnum: Object.fromEntries(
        AGE_LEVEL_OPTIONS.map((lv) => [lv, { text: lv }]),
      ),
    },
    {
      title: '舞种',
      dataIndex: 'dance_type',
      width: 100,
      valueEnum: Object.fromEntries(
        Object.entries(DANCE_TYPE_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, record) => DANCE_TYPE_LABELS[record.dance_type],
    },
    { title: '选手姓名', dataIndex: 'player_name', width: 100 },
    {
      title: '证件号',
      dataIndex: 'id_card_masked',
      width: 170,
      search: false,
      render: (_, record) => record.id_card_masked ?? '无证件号',
    },
    {
      title: '金额',
      dataIndex: 'amount',
      width: 110,
      search: false,
      render: (_, record) => formatMoney(record.amount),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      valueEnum: Object.fromEntries(
        Object.entries(ORDER_STATUS_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, record) => (
        <Tag color={STATUS_COLORS[record.status]}>
          {ORDER_STATUS_LABELS[record.status]}
          {record.status === 'canceled' ? '（留档）' : ''}
        </Tag>
      ),
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 90,
      valueEnum: Object.fromEntries(
        Object.entries(ORDER_SOURCE_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, record) => ORDER_SOURCE_LABELS[record.source],
    },
    {
      title: '支付时间',
      dataIndex: 'paid_at',
      width: 150,
      search: false,
      render: (_, record) => formatDateTime(record.paid_at),
    },
    {
      title: '支付时间',
      dataIndex: 'paid_range',
      hideInTable: true,
      valueType: 'dateTimeRange',
      search: { transform: (value) => ({ paid_from: value?.[0], paid_to: value?.[1] }) },
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 150,
      search: false,
      render: (_, record) => formatDateTime(record.created_at),
    },
    {
      title: '操作',
      valueType: 'option',
      width: 140,
      render: (_, record) => (
        <Space size={4}>
          <Typography.Link
            onClick={() => history.push(`/order/detail/${record.order_no}`)}
          >
            查看详情
          </Typography.Link>
          {record.status === 'paid' && (
            <Typography.Link
              onClick={() =>
                history.push(`/order/refunds?keyword=${record.order_no}`)
              }
            >
              发起退款
            </Typography.Link>
          )}
        </Space>
      ),
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<OrderBrief>
        headerTitle="订单管理"
        rowKey="order_no"
        actionRef={actionRef}
        columns={columns}
        form={{ initialValues: { event_id: urlEventId } }}
        search={{ labelWidth: 'auto' }}
        request={async (params) => {
          const paidRange = params.paid_range as [string, string] | undefined;
          const data = await listOrders({
            event_id: params.event_id as number | undefined,
            level: params.level as AgeLevel | undefined,
            dance_type: params.dance_type as DanceType | undefined,
            status: params.status as OrderStatus | undefined,
            source: params.source as OrderSource | undefined,
            include_canceled: includeCanceled,
            paid_from: paidRange?.[0],
            paid_to: paidRange?.[1],
            keyword: params.order_no || params.player_name,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        toolBarRender={() => [
          <span key="canceled">
            显示已取消（对账）
            <Switch
              style={{ marginLeft: 8, marginRight: 16 }}
              checked={includeCanceled}
              onChange={(checked) => {
                setIncludeCanceled(checked);
                actionRef.current?.reload();
              }}
            />
          </span>,
          <Button key="export" onClick={handleExport}>
            导出
          </Button>,
        ]}
      />
    </PageContainer>
  );
};

export default OrderList;
