import type { ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProFormSelect, ProTable } from '@ant-design/pro-components';
import { useService } from '@/services/youwumeng/useService';
import { Alert, Card, Tag, Typography } from 'antd';
import React, { useState } from 'react';
import { formatMoney } from '@/services/youwumeng/format';
import { getInstitutionRanking, listSeasons } from '@/services/youwumeng/seasons';
import type { InstitutionRankRow } from '@/services/youwumeng/typings';

/** 机构年度榜（设计文档 8.5） */
const InstitutionRanking: React.FC = () => {
  const [seasonId, setSeasonId] = useState<number | undefined>(1);
  const { data: seasons } = useService(listSeasons);
  const { data: rows, loading } = useService(
    () => getInstitutionRanking(seasonId ?? 1),
    { refreshDeps: [seasonId] },
  );

  const columns: ProColumns<InstitutionRankRow>[] = [
    {
      title: '排名',
      dataIndex: 'rank',
      width: 80,
      render: (_, r) => (r.rank <= 3 ? <Tag color="gold">第 {r.rank} 名</Tag> : `第 ${r.rank} 名`),
    },
    { title: '机构名称', dataIndex: 'institution' },
    {
      title: '年度总积分',
      dataIndex: 'total_points',
      width: 130,
      sorter: (a, b) => a.total_points - b.total_points,
    },
    { title: '上榜选手数', dataIndex: 'player_count', width: 110 },
    {
      title: '荣誉',
      dataIndex: 'honor',
      render: (_, r) =>
        r.honor === 'annual_elite' ? (
          <Tag color="gold">年度菁英机构奖 {formatMoney(r.prize_amount)}</Tag>
        ) : r.honor === 'annual_top10' ? (
          <Tag color="blue">年度十佳机构奖</Tag>
        ) : (
          '-'
        ),
    },
  ];

  return (
    <PageContainer title={false}>
      <Card
        size="small"
        title="机构年度榜"
        extra={
          <ProFormSelect
            noStyle
            fieldProps={{
              value: seasonId,
              onChange: (v: number) => setSeasonId(v),
              style: { minWidth: 160 },
            }}
            request={async () =>
              (seasons ?? []).map((s) => ({ label: s.name, value: s.season_id }))
            }
          />
        }
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="默认规则：机构年度总积分 = 旗下选手年度积分之和；第 1–3 名「年度菁英机构奖」各 10,000 元，第 4–10 名「年度十佳机构奖」。"
        />
        <ProTable<InstitutionRankRow>
          rowKey="rank"
          size="small"
          search={false}
          options={false}
          pagination={false}
          loading={loading}
          dataSource={rows ?? []}
          columns={columns}
          locale={{ emptyText: '暂无机构上榜' }}
        />
      </Card>
    </PageContainer>
  );
};

export default InstitutionRanking;
