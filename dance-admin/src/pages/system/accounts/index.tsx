import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ModalForm,
  PageContainer,
  ProFormSelect,
  ProFormText,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { App, Button, Popconfirm, Space, Tag, Typography } from 'antd';
import React, { useRef } from 'react';
import { ADMIN_ROLE_LABELS } from '@/services/youwumeng/labels';
import { formatDateTime } from '@/services/youwumeng/format';
import {
  createAccount,
  listAccounts,
  updateAccount,
} from '@/services/youwumeng/system';
import type { AdminAccount, AdminRole } from '@/services/youwumeng/typings';

/** 账号与权限（设计文档 10.1）：仅超管可操作 */
const Accounts: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const actionRef = useRef<ActionType>(null);

  const columns: ProColumns<AdminAccount>[] = [
    { title: '账号', dataIndex: 'username', width: 130 },
    { title: '姓名', dataIndex: 'name', width: 120 },
    {
      title: '角色',
      dataIndex: 'role',
      width: 110,
      render: (_, r) => (
        <Tag color={r.role === 'super_admin' ? 'gold' : r.role === 'admin' ? 'blue' : 'default'}>
          {ADMIN_ROLE_LABELS[r.role]}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      render: (_, r) =>
        r.status === 'active' ? <Tag color="success">启用</Tag> : <Tag color="error">已停用</Tag>,
    },
    { title: '最近登录', dataIndex: 'last_login_at', width: 150, render: (_, r) => formatDateTime(r.last_login_at) },
    { title: '创建时间', dataIndex: 'created_at', width: 150, render: (_, r) => formatDateTime(r.created_at) },
    {
      title: '操作',
      valueType: 'option',
      width: 240,
      render: (_, r) => (
        <Space size={8}>
          <ModalForm
            title={`修改角色：${r.name}`}
            trigger={<Typography.Link>修改角色</Typography.Link>}
            onFinish={async (values) => {
              await updateAccount(r.admin_id, { role: values.role as AdminRole });
              message.success('已修改');
              actionRef.current?.reload();
              return true;
            }}
          >
            <ProFormSelect
              name="role"
              label="角色"
              initialValue={r.role}
              rules={[{ required: true }]}
              options={Object.entries(ADMIN_ROLE_LABELS).map(([value, label]) => ({ value, label }))}
            />
          </ModalForm>
          <ModalForm
            title={`重置密码：${r.name}`}
            trigger={<Typography.Link>重置密码</Typography.Link>}
            onFinish={async (values) => {
              await updateAccount(r.admin_id, { password: values.password });
              message.success('密码已重置');
              return true;
            }}
          >
            <ProFormText.Password
              name="password"
              label="新密码"
              rules={[{ required: true, message: '请输入新密码' }]}
            />
          </ModalForm>
          <Popconfirm
            title={r.status === 'active' ? '停用账号' : '启用账号'}
            description="该账号将无法/恢复登录，操作留痕。"
            onConfirm={async () => {
              await updateAccount(r.admin_id, {
                status: r.status === 'active' ? 'disabled' : 'active',
              });
              message.success('已更新');
              actionRef.current?.reload();
            }}
          >
            <Typography.Link type={r.status === 'active' ? 'danger' : undefined}>
              {r.status === 'active' ? '停用' : '启用'}
            </Typography.Link>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<AdminAccount>
        headerTitle="账号与权限"
        rowKey="admin_id"
        actionRef={actionRef}
        columns={columns}
        search={false}
        options={false}
        request={async (params) => {
          const data = await listAccounts({ page: params.current, page_size: params.pageSize });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        toolBarRender={() =>
          access.isSuperAdmin
            ? [
                <ModalForm
                  key="create"
                  title="创建账号"
                  trigger={<Button type="primary">创建账号</Button>}
                  onFinish={async (values) => {
                    await createAccount({
                      username: values.username,
                      name: values.name,
                      role: values.role as AdminRole,
                      password: values.password,
                    });
                    message.success('已创建');
                    actionRef.current?.reload();
                    return true;
                  }}
                >
                  <ProFormText name="username" label="账号" rules={[{ required: true, message: '请输入账号' }]} />
                  <ProFormText name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]} />
                  <ProFormSelect
                    name="role"
                    label="角色"
                    initialValue="admin"
                    rules={[{ required: true }]}
                    options={Object.entries(ADMIN_ROLE_LABELS).map(([value, label]) => ({ value, label }))}
                  />
                  <ProFormText.Password
                    name="password"
                    label="初始密码"
                    rules={[{ required: true, message: '请设置初始密码' }]}
                  />
                </ModalForm>,
              ]
            : []
        }
      />
    </PageContainer>
  );
};

export default Accounts;
