import { Line } from '@ant-design/plots';
import { PageContainer, ProTable, StatisticCard } from '@ant-design/pro-components';
import { history } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import { Button, Card, Col, List, Progress, Row, Space, Typography } from 'antd';
import React from 'react';
import { getDashboard } from '@/services/youwumeng/dashboard';
import { formatMoney } from '@/services/youwumeng/format';

/**
 * 数据看板：核心指标卡 + 待办事项 + 报名监控 + 近 30 日趋势 + 快捷操作。
 * 对应设计文档 1 节。
 */
const Dashboard: React.FC = () => {
  const { data, loading } = useService(getDashboard);

  const trendConfig = {
    data: data?.trend ?? [],
    xField: 'date',
    yField: 'registrations',
    smooth: true,
    height: 280,
    point: { size: 0 },
    area: { style: { fill: 'l(270) 0:#ffffff 1:#b08d5733' } },
  };

  return (
    <PageContainer title={false} loading={loading && !data}>
      <Row gutter={16}>
        <Col span={4}>
          <StatisticCard
            statistic={{ title: '今日新增报名', value: data?.metrics.today_registrations ?? 0 }}
            onClick={() => history.push('/order/list')}
          />
        </Col>
        <Col span={4}>
          <StatisticCard
            statistic={{
              title: '今日实收金额',
              value: formatMoney(data?.metrics.today_revenue ?? 0),
            }}
          />
        </Col>
        <Col span={4}>
          <StatisticCard
            statistic={{
              title: '累计有效报名 / 实收',
              value: `${data?.metrics.total_registrations ?? 0} 笔`,
              description: formatMoney(data?.metrics.total_revenue ?? 0),
            }}
          />
        </Col>
        <Col span={4}>
          <StatisticCard
            statistic={{
              title: '待处理退款工单',
              value: data?.metrics.pending_refund_tickets ?? 0,
              valueStyle:
                (data?.metrics.pending_refund_tickets ?? 0) > 0
                  ? { color: '#cf1322' }
                  : undefined,
            }}
            onClick={() => history.push('/order/refunds')}
          />
        </Col>
        <Col span={4}>
          <StatisticCard
            statistic={{ title: '候补中人数', value: data?.metrics.waitlist_total ?? 0 }}
            onClick={() => history.push('/operation/waitlist')}
          />
        </Col>
        <Col span={4}>
          <StatisticCard
            statistic={{ title: '未传齐资料', value: data?.metrics.incomplete_materials ?? 0 }}
            onClick={() => history.push('/operation/materials')}
          />
        </Col>
      </Row>

      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={10}>
          <Card title="待我处理" size="small">
            <List
              size="small"
              dataSource={[
                `退款工单待处理：${data?.todos.pending_refunds ?? 0} 笔`,
                `客服新登记工单：${data?.todos.new_cs_tickets ?? 0} 条`,
                ...(data?.todos.closing_events ?? []).map(
                  (e) => `「${e.event_name}」${e.days_left} 天后截止报名`,
                ),
              ]}
              renderItem={(item) => <List.Item>{item}</List.Item>}
              locale={{ emptyText: '暂无待办' }}
            />
          </Card>
        </Col>
        <Col span={14}>
          <Card
            title="报名监控（进行中赛事组别进度）"
            size="small"
            extra={
              <Space>
                <Button onClick={() => history.push('/event/create')}>创建赛事</Button>
                <Button onClick={() => history.push('/order/list')}>导出名单</Button>
                <Button onClick={() => history.push('/order/refunds')}>处理退款</Button>
              </Space>
            }
          >
            <ProTable
              rowKey={(r) => `${r.event_id}-${r.level}`}
              size="small"
              search={false}
              options={false}
              pagination={false}
              dataSource={data?.event_progress ?? []}
              columns={[
                {
                  title: '赛事',
                  dataIndex: 'event_name',
                  render: (_, r) => (
                    <Typography.Link
                      onClick={() => history.push(`/event/detail/${r.event_id}`)}
                    >
                      {r.event_name}
                    </Typography.Link>
                  ),
                },
                { title: '组别', dataIndex: 'level', width: 140 },
                {
                  title: '报名进度',
                  hideInTable: true,
                },
                {
                  title: '',
                  width: 220,
                  render: (_, r) => (
                    <Progress
                      percent={Math.round((r.registered / Math.max(1, r.quota)) * 100)}
                      size="small"
                      status={
                        r.registered >= r.quota
                          ? 'exception'
                          : r.waitlist_count > 0
                            ? 'active'
                            : 'normal'
                      }
                    />
                  ),
                },
                {
                  title: '已报/名额',
                  width: 110,
                  render: (_, r) => (
                    <span style={{ color: r.registered >= r.quota ? '#cf1322' : undefined }}>
                      {r.registered}/{r.quota}
                      {r.registered >= r.quota ? '（满员）' : ''}
                    </span>
                  ),
                },
                { title: '候补', dataIndex: 'waitlist_count', width: 70 },
              ]}
            />
          </Card>
        </Col>
      </Row>

      <Card title="近 30 日报名趋势" size="small" style={{ marginTop: 16 }}>
        <Line {...trendConfig} />
      </Card>
    </PageContainer>
  );
};

export default Dashboard;
