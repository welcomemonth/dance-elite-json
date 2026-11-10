import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { Helmet, useModel } from '@umijs/max';
import { App } from 'antd';
import { createStyles } from 'antd-style';
import React, { startTransition } from 'react';
import { login, saveSession } from '@/services/youwumeng/auth';
import type { LoginParams } from '@/services/youwumeng/typings';
import Settings from '../../../../config/defaultSettings';

const useStyles = createStyles(({ token }) => {
  return {
    container: {
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      overflow: 'auto',
      backgroundImage:
        'linear-gradient(135deg, #f5f3ee 0%, #ece7dd 100%)',
    },
    loginCard: {
      background: token.colorBgContainer,
      borderRadius: token.borderRadiusLG,
      boxShadow: token.boxShadowTertiary,
      padding: '32px 40px',
    },
  };
});

/**
 * 校验 redirect 参数，防开放跳转（仅允许同源相对路径）。
 */
const getSafeRedirectUrl = (redirect: string | null): string => {
  if (!redirect?.startsWith('/')) return '/';
  if (redirect.startsWith('//')) return '/';
  try {
    const parsed = new URL(redirect, window.location.origin);
    if (parsed.origin !== window.location.origin) return '/';
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return '/';
  }
};

const Login: React.FC = () => {
  const { styles } = useStyles();
  const { message } = App.useApp();
  const { setInitialState } = useModel('@@initialState');

  const handleSubmit = async (values: LoginParams) => {
    try {
      const result = await login(values);
      saveSession(result.token, result.admin);
      startTransition(() => {
        setInitialState((s) => ({ ...s, currentUser: result.admin }));
      });
      message.success('登录成功');
      const urlParams = new URL(window.location.href).searchParams;
      window.location.href = getSafeRedirectUrl(urlParams.get('redirect'));
    } catch {
      // 错误提示由全局 errorHandler（40102 账号或密码错误等）统一处理
    }
  };

  return (
    <div className={styles.container}>
      <Helmet>
        <title>{`登录 - ${Settings.title}`}</title>
      </Helmet>
      <div
        style={{
          flex: '1',
          padding: '64px 0',
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'center',
        }}
      >
        <div className={styles.loginCard}>
          <LoginForm
            contentStyle={{ minWidth: 320 }}
            logo={<img alt="logo" src="/logo.svg" />}
            title="优舞盟"
            subTitle="少儿中国舞赛事后台管理系统"
            onFinish={async (values) => {
              await handleSubmit(values as LoginParams);
            }}
          >
            <ProFormText
              name="username"
              fieldProps={{ size: 'large', prefix: <UserOutlined /> }}
              placeholder="账号"
              rules={[{ required: true, message: '请输入账号' }]}
            />
            <ProFormText.Password
              name="password"
              fieldProps={{ size: 'large', prefix: <LockOutlined /> }}
              placeholder="密码"
              rules={[{ required: true, message: '请输入密码' }]}
            />
          </LoginForm>
        </div>
      </div>
    </div>
  );
};

export default Login;
