import { LayoutGrid, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";

export function EmptyBoard({
  filtered,
  onCreate,
  onResetFilter,
}: {
  filtered: boolean;
  onCreate: () => void;
  onResetFilter: () => void;
}) {
  if (filtered) {
    return (
      <Empty className="mx-auto max-w-md py-20">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <LayoutGrid />
          </EmptyMedia>
          <EmptyTitle>該当するアプリがありません</EmptyTitle>
          <EmptyDescription>検索やサイドバーの絞り込みを変えてみてください。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button variant="outline" size="sm" onClick={onResetFilter}>
            条件をクリア
          </Button>
        </EmptyContent>
      </Empty>
    );
  }

  return (
    <Empty className="mx-auto max-w-md py-20">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Plus />
        </EmptyMedia>
        <EmptyTitle>最初のアプリを登録</EmptyTitle>
        <EmptyDescription>
          フォルダと起動コマンドを書けば、あとは起動と終了だけです。ポートは自動でも、決めても構いません。
        </EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button onClick={onCreate}>
          <Plus data-icon="inline-start" />
          アプリを登録
        </Button>
      </EmptyContent>
    </Empty>
  );
}
