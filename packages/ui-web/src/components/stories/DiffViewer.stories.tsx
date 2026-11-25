import type { Meta, StoryObj } from "@storybook/react";
import { expect, within } from "@storybook/test";
import { DiffViewer } from "../DiffViewer";

const meta: Meta<typeof DiffViewer> = {
  component: DiffViewer,
  title: "@agent-status/ui-web/components/DiffViewer",
};

export default meta;

type Story = StoryObj<typeof DiffViewer>;

export const SingleHunkEdit: Story = {
  args: {
    payload: {
      action: "EDIT",
      filePath: "src/components/LogDrawer.tsx",
      hunks: [
        {
          header: "@@ -1,3 +1,3 @@",
          lines: [
            { type: "ctx", oldLineNumber: 1, newLineNumber: 1, content: "import React from 'react';" },
            {
              type: "del",
              oldLineNumber: 2,
              content: "export function LogDrawerOld() {",
              wordTokens: [
                { type: "same", value: "export function " },
                { type: "del", value: "LogDrawerOld" },
                { type: "same", value: "() {" },
              ],
            },
            {
              type: "add",
              newLineNumber: 2,
              content: "export function LogDrawer() {",
              wordTokens: [
                { type: "same", value: "export function " },
                { type: "add", value: "LogDrawer" },
                { type: "same", value: "() {" },
              ],
            },
            { type: "ctx", oldLineNumber: 3, newLineNumber: 3, content: "  return <div />;" },
          ],
        },
      ],
    },
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const viewer = canvas.getByTestId("DiffViewer");
    await expect(viewer).toBeInTheDocument();
    await expect(canvas.getByTestId("DiffViewer--badge")).toHaveTextContent("EDIT");
    await expect(canvas.getByTestId("DiffViewer--path")).toHaveTextContent("src/components/LogDrawer.tsx");
    await expect(canvas.getByTestId("DiffViewer--del-line")).toBeInTheDocument();
    await expect(canvas.getByTestId("DiffViewer--add-line")).toBeInTheDocument();
  },
};

export const MultiHunkEdit: Story = {
  args: {
    payload: {
      action: "EDIT",
      filePath: "internal/classifier/classifier.go",
      hunks: [
        {
          header: "Hunk 1 of 2",
          lines: [
            { type: "del", oldLineNumber: 15, content: "const DefaultTimeout = 10" },
            { type: "add", newLineNumber: 15, content: "const DefaultTimeout = 30" },
          ],
        },
        {
          header: "Hunk 2 of 2",
          lines: [
            { type: "ctx", oldLineNumber: 50, newLineNumber: 50, content: "func Process() error {" },
            { type: "add", newLineNumber: 51, content: "\tlog.Println('started')" },
            { type: "ctx", oldLineNumber: 51, newLineNumber: 52, content: "\treturn nil" },
          ],
        },
      ],
    },
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const viewer = canvas.getByTestId("DiffViewer");
    await expect(viewer).toBeInTheDocument();
    const headers = canvas.getAllByTestId("DiffViewer--hunk-header");
    await expect(headers).toHaveLength(2);
    await expect(headers[0]).toHaveTextContent("Hunk 1 of 2");
    await expect(headers[1]).toHaveTextContent("Hunk 2 of 2");
  },
};

export const WriteFile: Story = {
  args: {
    payload: {
      action: "WRITE",
      filePath: "src/types/auth.ts",
      content: "export interface IUser {\n  id: string;\n  name: string;\n}\n",
    },
  },
  async play({ canvasElement }) {
    const canvas = within(canvasElement);
    const viewer = canvas.getByTestId("DiffViewer");
    await expect(viewer).toBeInTheDocument();
    await expect(canvas.getByTestId("DiffViewer--badge")).toHaveTextContent("WRITE");
    await expect(canvas.getByTestId("DiffViewer--write-content")).toBeInTheDocument();
    await expect(canvas.getByText("export interface IUser {")).toBeInTheDocument();
  },
};
