import { useState } from 'react'
import { Button, Card, Form, Input, Typography } from 'antd'

import { useTheme } from '../theme/ThemeContext'
import { useAuth } from './AuthContext'
import './LoginScreen.css'

type LoginFormValues = {
  username: string
  password: string
}

export function LoginScreen() {
  const { login } = useAuth()
  const { theme, toggleTheme } = useTheme()
  const [form] = Form.useForm<LoginFormValues>()
  const [submitting, setSubmitting] = useState(false)

  async function submit(values: LoginFormValues) {
    setSubmitting(true)
    try {
      await login(values)
    } catch (cause) {
      form.setFields([
        {
          name: 'password',
          errors: [cause instanceof Error ? cause.message : String(cause)],
        },
      ])
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="login-screen">
      <div className="login-screen-panel">
        <div className="login-screen-header">
          <Typography.Title level={2} className="login-screen-title">
            Metarr
          </Typography.Title>
          <Typography.Text type="secondary">Sign in with the admin account</Typography.Text>
        </div>

        <Card>
          <Form form={form} layout="vertical" onFinish={(values) => void submit(values)}>
            <Form.Item
              name="username"
              label="Username"
              rules={[{ required: true, message: 'Username is required' }]}
            >
              <Input autoComplete="username" autoFocus />
            </Form.Item>

            <Form.Item
              name="password"
              label="Password"
              rules={[{ required: true, message: 'Password is required' }]}
            >
              <Input.Password autoComplete="current-password" />
            </Form.Item>

            <Form.Item style={{ marginBottom: 0 }}>
              <Button type="primary" htmlType="submit" block loading={submitting}>
                Sign in
              </Button>
            </Form.Item>
          </Form>
        </Card>

        <Button type="text" block className="login-screen-theme-toggle" onClick={toggleTheme}>
          Switch to Solarized {theme === 'dark' ? 'Light' : 'Dark'}
        </Button>
      </div>
    </div>
  )
}
