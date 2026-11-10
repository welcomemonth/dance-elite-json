import { PlusOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import {
  ModalForm,
  PageContainer,
  ProFormDatePicker,
  ProFormText,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import { App, Button, Popconfirm, Tag } from 'antd';
import React, { useState } from 'react';
import { formatDate } from '@/services/youwumeng/format';
import { SEASON_STATUS_LABELS, boardLabel } from '@/services/youwumeng/labels';
import { archiveSeason, createSeason, listSeasons, updateSeason } from '@/services/youwumeng/seasons';
import type { Season } from '@/services/youwumeng/typings';

const STATUS_COLORS: Record<Season['status'], string> = {
  upcoming: 'default',
  ongoing: 'success',
  archived: 'warning',
};

const SeasonList: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const [editing, setEditing] = useState<Season | null>(null);

  const { data, loading, refresh } = useService(listSeasons);

  const { run: runCreate } = useService(createSeason, {
    manual: true,
    onSuccess: () => {
      message.success('赛季已创建');
      refresh();
    },
  });
  const { run: runUpdate } = useService(updateSeason, {
    manual: true,
    onSuccess: () => {
      message.success('已保存');
      setEditing(null);
      refresh();
    },
  });
  const { run: runArchive } = useService(archiveSeason, {
    manual: true,
    onSuccess: () => {
      message.success('赛季已归档，榜单冻结为历史');
      refresh();
    },
  });

  const columns: ProColumns<Season>[] = [
    { title: '赛季名称', dataIndex: 'name' },
    {
      title: '起止时间',
      width: 220,
      render: (_, r) => `${formatDate(r.start_date)} ~ ${formatDate(r.end_date)}`,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      render: (_, r) => (
        <Tag color={STATUS_COLORS[r.status]}>{SEASON_STATUS_LABELS[r.status]}</Tag>
      ),
    },
    {
      title: '适用榜单',
      width: 320,
      render: (_, r) =>
        r.boards.map((b) => (
          <Tag key={`${b.level}-${b.dance_type}`}>{boardLabel(b.level, b.dance_type)}</Tag>
        )),
    },
    {
      title: '操作',
      width: 140,
      render: (_, r) =>
        access.canAdmin ? (
          <>
            <Button
              type="link"
              size="small"
              onClick={() => setEditing(r)}
            >
              编辑
            </Button>
            {r.status !== 'archived' && (
              <Popconfirm
                title="归档赛季"
                description="归档后榜单冻结为历史，小程序端可切换查看。是否继续？"
                onConfirm={() => runArchive(r.season_id)}
              >
                <Button type="link" size="small" danger>
                  归档
                </Button>
              </Popconfirm>
            )}
          </>
        ) : null,
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<Season>
        headerTitle="赛季管理"
        rowKey="season_id"
        loading={loading}
        dataSource={data ?? []}
        columns={columns}
        search={false}
        pagination={false}
        toolBarRender={() =>
          access.canAdmin
            ? [
                <ModalForm
                  key="create"
                  title="新建赛季"
                  trigger={
                    <Button type="primary" icon={<PlusOutlined />}>
                      新建赛季
                    </Button>
                  }
                  modalProps={{ destroyOnHidden: true }}
                  onFinish={async (values: { name: string; start_date: string; end_date: string }) => {
                    await runCreate(values);
                    return true;
                  }}
                >
                  <ProFormText
                    name="name"
                    label="赛季名称"
                    placeholder="如 2027 赛季"
                    rules={[{ required: true, message: '请输入赛季名称' }]}
                  />
                  <ProFormDatePicker
                    name="start_date"
                    label="开始日期"
                    rules={[{ required: true, message: '请选择开始日期' }]}
                  />
                  <ProFormDatePicker
                    name="end_date"
                    label="结束日期"
                    rules={[{ required: true, message: '请选择结束日期' }]}
                  />
                </ModalForm>,
              ]
            : []
        }
      />

      <ModalForm
        title={`编辑赛季 - ${editing?.name ?? ''}`}
        open={!!editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
        initialValues={editing ?? undefined}
        modalProps={{ destroyOnHidden: true }}
        onFinish={async (values: { name: string; start_date: string; end_date: string }) => {
          if (!editing) return false;
          await runUpdate(editing.season_id, values);
          return true;
        }}
      >
        <ProFormText
          name="name"
          label="赛季名称"
          rules={[{ required: true, message: '请输入赛季名称' }]}
        />
        <ProFormDatePicker
          name="start_date"
          label="开始日期"
          rules={[{ required: true, message: '请选择开始日期' }]}
        />
        <ProFormDatePicker
          name="end_date"
          label="结束日期"
          rules={[{ required: true, message: '请选择结束日期' }]}
        />
      </ModalForm>
    </PageContainer>
  );
};

export default SeasonList;
