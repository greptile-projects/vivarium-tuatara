import { ServiceObjectivesWorkspace } from "@/components/service-objectives-workspace";

export default async function ReliabilityPage({params}:{params:Promise<{id:string}>}){const {id}=await params;return <ServiceObjectivesWorkspace repositoryID={id}/>}
