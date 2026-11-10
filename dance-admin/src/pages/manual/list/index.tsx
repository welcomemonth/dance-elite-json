import { PlusOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history, useAccess } from '@umijs/max';
import { Button, Tag, Typography } from 'antd';
import React from 'react';
import { formatDateTime, formatMoney } from '@/services/youwumeng/format';
import { DANCE_TYPE_LABELS, ORDER_STATUS_LABELS } from '@/services/youwumeng/labels';
import { listOrders } from '@/services/youwumeng/orders';
import type { OrderBrief, OrderStatus } from '@/services/youwumeng/typings';

const STATUS_COLORS: Record<OrderStatus, string> = {
  paid: 'success',
  canceled: 'default',
  refunding: 'warning',
  refunded: 'processing',
};

/**
 * 人工报名列表（特殊通道）：与订单同库，来源 = 后台人工。
 * 对应设计文档 5.1 节。
 */
const ManualList: React.FC = () => {
  const access = useAccess();

  const columns: ProColumns<OrderBrief>[] = [
    {
      title: '订单号',
      dataIndex: 'order_no',
      width: 210,
      render: (_, record) => (
        <Typography.Link
          onClick={() => history.push(`/order/detail/${record.order_no}`)}
        >
          {record.order_no}
        </Typography.Link>
      ),
    },
    { title: '赛事', dataIndex: 'event_name', width: 260, ellipsis: true },
    { title: '选手姓名', dataIndex: 'player_name', width: 100 },
    {
      title: '组别',
      dataIndex: 'level',
      width: 150,
      search: false,
      render: (_, record) =>
        `${record.level}·${DANCE_TYPE_LABELS[record.dance_type]}`,
    },
    {
      title: '金额',
      dataIndex: 'amount',
      width: 110,
      search: false,
      render: (_, record) =>
        record.amount === 0 ? <Tag>免费</Tag> : formatMoney(record.amount),
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
        </Tag>
      ),
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
      width: 100,
      render: (_, record) => [
        <Typography.Link
          key="detail"
          onClick={() => history.push(`/order/detail/${record.order_no}`)}
        >
          查看详情
        </Typography.Link>,
      ],
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<OrderBrief>
        headerTitle="人工报名"
        rowKey="order_no"
        columns={columns}
        search={false}
        request={async (params) => {
          const data = await listOrders({
            source: 'manual',
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        toolBarRender={() =>
          access.canAdmin
            ? [
                <Button
                  key="create"
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => history.push('/order/manual/create')}
                >
                  新建人工报名
                </Button>,
              ]
            : []
        }
      />
    </PageContainer>
  );
};

export default ManualList;
