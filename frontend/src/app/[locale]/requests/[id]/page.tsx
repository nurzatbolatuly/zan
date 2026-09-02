import { RequestStatusView } from '../../../../components/RequestStatusView';

export default async function RequestStatusPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <RequestStatusView requestId={id} />;
}
